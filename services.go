// Package colony implements a lightweight microservice framework on top of NSQ.
package colony

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bitly/go-nsq"
)

// Topic contains the components of an NSQ Topic used for communication between
// services
type Topic struct {
	ServiceName string
	ServiceID   string
	ContentType string
}

// GetName returns the properly formatted topic name from a Topic
func (t Topic) GetName() string {
	return t.ServiceName + "-" + t.ServiceID + "-" + t.ContentType
}

type MessageID string

// A Message wraps a payload of data, and contains everything required for
// successful routing through NSQ between services
type Message struct {
	Topic         Topic     // topic message appears on
	FromName      string    // name of originating service
	Payload       []byte    // actual message content
	Time          time.Time // time message was generated
	ResponseTopic Topic     // responses to this message can be sent here
	ContentType   string    // contentType of message
	MessageID     MessageID // message id
}

// Handler is the message processing interface for any service.
type Handler interface {
	HandleMessage(Message) error
}

type handlerIDPair struct {
	h  HandlerFunc
	id MessageID
}

type channelIDPair struct {
	c  chan Message
	id MessageID
}

// HandlerFuncs live in the service and handles messages that are responding to the
// service
type HandlerFunc func(chan Message) error

// Service contains all the information for a service necessary for successful
// routing of messages to and from that service
type Service struct {
	Name                string
	ID                  string
	i                   int // this is just for IDs #TODO make this not crap
	handlers            map[MessageID]HandlerFunc
	activeHandlers      map[MessageID]chan Message
	addHandlerChan      chan handlerIDPair
	activateHandlerChan chan channelIDPair
	removeHandlerChan   chan MessageID
	callHandlerChan     chan Message
	producer            *nsq.Producer
	nsqLookupdAddr      string
	nsqLookupdHTTPAddr  string
	nsqdAddr            string
	nsqdHTTPAddr        string
	responseTopic       Topic
}

// NewService returns a service associated with a specific NSQ daemon.
func NewService(name, id, nsqLookupdAddr, nsqLookupdHTTPAddr, nsqdAddr, nsqdHTTPAddr string) *Service {
	conf := nsq.NewConfig()
	err := conf.Set("lookupd_poll_interval", "5s")
	producer, err := nsq.NewProducer(nsqdAddr, conf)
	if err != nil {
		log.Fatal(err.Error())
	}
	responseTopic := Topic{
		ServiceName: name,
		ServiceID:   id,
		ContentType: "responses",
	}
	s := &Service{
		Name:                name,
		ID:                  id,
		handlers:            make(map[MessageID]HandlerFunc),
		activeHandlers:      make(map[MessageID]chan Message),
		addHandlerChan:      make(chan handlerIDPair),
		activateHandlerChan: make(chan channelIDPair),
		removeHandlerChan:   make(chan MessageID),
		callHandlerChan:     make(chan Message),
		producer:            producer,
		nsqLookupdAddr:      nsqLookupdAddr,
		nsqLookupdHTTPAddr:  nsqLookupdHTTPAddr,
		nsqdAddr:            nsqdAddr,
		nsqdHTTPAddr:        nsqdHTTPAddr,
		responseTopic:       responseTopic,
	}
	go s.start()
	return s
}

// start starts a service. This should be called once, probably inside its own
// goroutine.
func (s Service) start() {
	// initialise the response topic and start listening
	go s.responseHandler()
	// manage response handlers
	for {
		select {
		case pair := <-s.addHandlerChan:
			s.handlers[pair.id] = pair.h
			log.Println("added handler for message with id", pair.id)
		case pair := <-s.activateHandlerChan:
			s.activeHandlers[pair.id] = pair.c
			log.Println("activated handler for message with id", pair.id)
		case id := <-s.removeHandlerChan:
			delete(s.handlers, id)
			log.Println("removed handler for message with id", id)
		case msg := <-s.callHandlerChan:
			// try active handlers first
			c, ok := s.activeHandlers[msg.MessageID]
			if ok {
				log.Println("sending message to active handler for message", msg.MessageID)
				c <- msg
				continue
			}
			log.Println("activating handler for message with id", msg.MessageID)
			handler, ok := s.handlers[msg.MessageID]
			if !ok {
				log.Println("could not find message handler for id", msg.MessageID)
				continue
			}
			go func() {
				c = make(chan Message, 1)
				pair := channelIDPair{
					c:  c,
					id: msg.MessageID,
				}
				s.activateHandlerChan <- pair
				c <- msg
				err := handler(c)
				if err != nil {
					log.Println(err.Error())
				}
				log.Println("removing handler for message", msg.MessageID)
				delete(s.handlers, msg.MessageID)
				delete(s.activeHandlers, msg.MessageID)
			}()
		}
	}
}

// NewMessage creates a new message. Use Produce to emit this message to the
// network.
func (s *Service) NewMessage(contentType string, payload []byte) Message {
	from := Topic{
		ServiceName: s.Name,
		ServiceID:   s.ID,
		ContentType: contentType,
	}

	return Message{
		Topic:         from,
		FromName:      s.Name,
		Payload:       payload,
		Time:          time.Now(),
		ResponseTopic: s.responseTopic,
		MessageID:     s.nextID(),
		ContentType:   contentType,
	}
}

// This builds a message specifically as a response to an earlier message. Use
// Producer to send this to the originating service.
func (s *Service) NewResponse(m Message, contentType string, payload []byte) Message {
	return Message{
		Topic:         m.ResponseTopic,
		FromName:      s.Name,
		Payload:       payload,
		Time:          time.Now(),
		ResponseTopic: s.responseTopic,
		MessageID:     m.MessageID,
		ContentType:   contentType,
	}
}

func (s *Service) nextID() MessageID {
	s.i = s.i + 1
	return MessageID(strconv.Itoa(s.i))
}

type createTopicResponse struct {
	Status_code int
	Status_txt  string
	Data        string
}

func (s *Service) createTopic(topic string) error {
	resp, err := http.Get("http://" + s.nsqdHTTPAddr + "/create_topic?topic=" + topic)
	if err != nil {
		log.Fatal(err.Error())
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	var r createTopicResponse
	json.Unmarshal(body, &r)
	if r.Status_code != 200 {
		log.Println(r)
		return errors.New("could not creat topic " + topic)
	}
	return nil
}

func (s Service) HandleMessage(m *nsq.Message) error {
	var out Message
	err := json.Unmarshal(m.Body, &out)
	if err != nil {
		return err
	}
	s.callHandlerChan <- out
	return nil
}

func (s *Service) responseHandler() {
	// initialise response topic
	channelName := s.Name + "-" + s.ID + "-responseHandler"
	log.Println("CHANNEL NAME", channelName)

	conf := nsq.NewConfig()
	err := conf.Set("lookupd_poll_interval", "5s")
	if err != nil {
		log.Fatal(err.Error())
	}
	err = s.createTopic(s.responseTopic.GetName())
	if err != nil {
		log.Fatal(err.Error())
	}

	topicName := s.responseTopic.GetName()
	log.Println("about to start consumer on Topic", topicName, "with channel ->"+channelName+"<-")
	log.Println(nsq.IsValidChannelName(channelName))
	log.Println(channelName)
	c, err := nsq.NewConsumer(topicName, channelName, conf)
	log.Println("HELLO")
	if err != nil {
		log.Fatal(err.Error())
	}
	c.SetHandler(s)
	c.ConnectToNSQLookupd(s.nsqLookupdHTTPAddr)
}

// Produce emits a message to the netowrk on the appropriate topic. If the
// handler is not nil, then the handler is registered with the service for
// responses to this message.
func (s Service) Produce(m Message, h HandlerFunc) error {
	if h != nil {
		s.addHandlerChan <- handlerIDPair{
			h:  h,
			id: m.MessageID,
		}
	}
	topic := m.Topic.GetName()
	out, err := json.Marshal(m)
	if err != nil {
		log.Fatal(err.Error())
	}
	s.producer.Publish(topic, out)
	return nil
}

type queueConsumer struct {
	C chan Message
}

func (c queueConsumer) HandleMessage(m *nsq.Message) error {
	var out Message
	err := json.Unmarshal(m.Body, &out)
	if err != nil {
		log.Fatal(err.Error())
	}
	c.C <- out
	return nil
}

// A Consumer consumes data from the network of a specific contentType. Any
// messages that appear in the network of this contentType will be routed to the
// Consumer's channel.
type Consumer struct {
	C           <-chan Message
	ContentType string
}

type lookupdTopics struct {
	Topics []string
}

type lookupdTopic struct {
	Status_code int
	Status_txt  string
	Data        lookupdTopics
}

func (s Service) lookupTopics(contentType string) []string {
	resp, err := http.Get("http://" + s.nsqLookupdHTTPAddr + "/topics")
	if err != nil {
		log.Fatal(err.Error())
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	log.Println("body", string(body))
	var t lookupdTopic
	err = json.Unmarshal(body, &t)
	if err != nil {
		log.Fatal(err.Error())
	}
	var out []string
	for _, topic := range t.Data.Topics {
		if strings.HasSuffix(topic, contentType) {
			out = append(out, topic)
		}
	}
	return out
}

// NewConsumer returns a colony Consumer of the specified contentType. The new
// Consumer is hooked up and ready to go - messages will appear immediately on
// its channel.
func (s Service) NewConsumer(contentType string) Consumer {
	inbound := make(chan Message)
	conf := nsq.NewConfig()

	consumer := Consumer{
		C:           inbound,
		ContentType: contentType,
	}

	// find existing topcis of that contetType
	topicsToConsume := s.lookupTopics(contentType)

	channel := s.Name + "-" + s.ID
	// create a consumer for each topic that matches
	for _, topic := range topicsToConsume {
		c, err := nsq.NewConsumer(topic, channel, conf)
		if err != nil {
			log.Fatal(err.Error())
		}
		c.SetHandler(queueConsumer{
			C: inbound,
		})
		c.ConnectToNSQLookupd(s.nsqLookupdHTTPAddr)
	}
	return consumer
}
