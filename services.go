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

// topic contains the components of an NSQ topic used for communication between
// services
type topic struct {
	ServiceName string
	ServiceID   string
	ContentType string
}

// getName returns the properly formatted topic name from a topic
func (t topic) getName() string {
	return t.ServiceName + "-" + t.ServiceID + "-" + t.ContentType
}

type messageID string

// A Message wraps a payload of data, and contains everything required for
// successful routing through NSQ between services. Generally
// NewMessage should be used to generate outbound messages and NewResponse to generate responses.
type Message struct {
	FromName      string    // name of originating service
	Payload       []byte    // actual message content
	Time          time.Time // time message was generated
	ContentType   string    // contentType of message
	MessageID     messageID // message id
	Topic         topic     // topic message appears on
	ResponseTopic topic     // responses to this message can be sent here
}

type handlerIDPair struct {
	h  Handler
	id messageID
}

// Handlers recieve a stream of Messages over the suppled channel
// in response to a corresponding outbound message. Each service needs
// to provide a Handler for each content type it consumes, and for
// each outbound message that can be responded to.
type Handler func(<-chan Message) error

// Service contains all the information for a service necessary for successful
// routing of messages to and from that service. To initialise a service use NewService.
type Service struct {
	Name               string // Name of the service
	ID                 string // ID of the service
	i                  int    // this is just for IDs #TODO make this not crap
	handlers           map[messageID]chan Message
	addHandlerChan     chan handlerIDPair
	callHandlerChan    chan Message
	producer           *nsq.Producer
	nsqLookupdHTTPAddr string
	nsqdAddr           string
	nsqdHTTPAddr       string
	responseTopic      topic
}

// NewService returns a colony service associated with a specific NSQ setup.
// Provide NSQ's lookupd HTTP address, and both the NSQ
// daemon's TCP and HTTP addresses. If running the default NSQ these
// ports are :4161, :4150, and :4151 respectively.
func NewService(name, id, nsqLookupdHTTPAddr, nsqdAddr, nsqdHTTPAddr string) *Service {
	conf := nsq.NewConfig()
	err := conf.Set("lookupd_poll_interval", "5s")
	producer, err := nsq.NewProducer(nsqdAddr, conf)
	if err != nil {
		log.Fatal(err.Error())
	}
	responseTopic := topic{
		ServiceName: name,
		ServiceID:   id,
		ContentType: "responses",
	}
	s := &Service{
		Name:               name,
		ID:                 id,
		handlers:           make(map[messageID]chan Message),
		addHandlerChan:     make(chan handlerIDPair),
		callHandlerChan:    make(chan Message),
		producer:           producer,
		nsqLookupdHTTPAddr: nsqLookupdHTTPAddr,
		nsqdAddr:           nsqdAddr,
		nsqdHTTPAddr:       nsqdHTTPAddr,
		responseTopic:      responseTopic,
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
			// make the channel that will be sent to the handler
			c := make(chan Message)
			// add the channel to our handler map
			s.handlers[pair.id] = c
			log.Println("activated handler for message with id", pair.id)
			// set the handler going
			go func() {
				err := pair.h(c)
				if err != nil {
					log.Println(err.Error())
				}
				// once the handler is complete, delete it from the handler map
				log.Println("removing handler for message", pair.id)
				delete(s.handlers, pair.id)
			}()
		case msg := <-s.callHandlerChan:
			c, ok := s.handlers[msg.MessageID]
			if !ok {
				log.Println("could not find message handler for id", msg.MessageID)
				continue
			}
			c <- msg
		}
	}
}

// NewMessage creates a new colony Message. Use Emit to emit this message to the
// network.
func (s *Service) NewMessage(contentType string, payload []byte) Message {
	from := topic{
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

// This builds a colony Message specifically as a response to a recieved Message. Use
// Request to send this to the originating service.
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

func (s *Service) nextID() messageID {
	s.i = s.i + 1
	return messageID(strconv.Itoa(s.i))
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

// The service's NSQ Handler routes messages from the service's response topic
// to the appopriate Handler. This function can be safely ignored when building a service.
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
	err = s.createTopic(s.responseTopic.getName())
	if err != nil {
		log.Fatal(err.Error())
	}

	topicName := s.responseTopic.getName()
	log.Println("about to start consumer on topic", topicName, "with channel ->"+channelName+"<-")
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

// Announce the production of a new content type to the colony, to alert existing services.
// If Announce is not called, only new services will discover this contentType.
func (s Service) Announce(contentType string) error {
	topicToAnnounce := topic{
		ServiceName: s.Name,
		ServiceID:   s.ID,
		ContentType: contentType,
	}
	m := Message{
		FromName:    s.Name,
		Time:        time.Now(),
		ContentType: contentType,
		Topic:       topicToAnnounce,
	}
	out, err := json.Marshal(m)
	if err != nil {
		log.Fatal(err.Error())
	}
	s.createTopic(topicToAnnounce.getName())
	s.producer.Publish("colony-announce", out)
	return nil
}

// Emit sends a Message from the service to the colony
func (s Service) Emit(m Message) error {
	return s.produce(m, nil)
}

// Request sends a Message from the service to the colony and specifies a
// Handler that will recieve the stream of responses.
func (s Service) Request(m Message, h Handler) error {
	return s.produce(m, h)
}

// produce emits a colony Message to the netowrk on the appropriate topic. If the
// Handler is not nil, then it is registered with the service for
// responses to this message.
func (s Service) produce(m Message, h Handler) error {
	if h != nil {
		s.addHandlerChan <- handlerIDPair{
			h:  h,
			id: m.MessageID,
		}
	}
	log.Println("MESSAGE TOPIC", m.Topic)
	topic := m.Topic.getName()
	out, err := json.Marshal(m)
	if err != nil {
		log.Fatal(err.Error())
	}
	s.producer.Publish(topic, out)
	return nil
}

// Consume registers the supplied Handler as a reciever of colony Messages of the specified contentType.
// When the Handler returns the service will no longer recieve messages of this type.
func (s Service) Consume(contentType string, h Handler) error {
	consumer := s.newConsumer(contentType)
	h(consumer.C)
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

// A consumer consumes data from the network of a specific contentType. Any
// messages that appear in the network of this contentType will be routed to the
// consumer's channel. Use newConsumer to generate consumers. When authoring a
// service use the service's Consume method.
type consumer struct {
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

// newConsumer returns a colony consumer of the specified contentType. The new
// consumer is hooked up and ready to go - messages will appear immediately on
// its channel.
func (s Service) newConsumer(contentType string) consumer {
	inbound := make(chan Message)
	conf := nsq.NewConfig()

	consumer := consumer{
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

	// begin the watch for new topics of this content type
	go s.watchForContentType(contentType, inbound)

	// return the consumer to the caller
	return consumer
}

func (s Service) watchForContentType(contentType string, inbound chan Message) {
	channel := s.Name + "-" + s.ID + "-" + contentType

	s.createTopic("colony-announce") // just in case

	conf := nsq.NewConfig()

	// connect to the colonly-announce topic
	c, err := nsq.NewConsumer("colony-announce", channel, conf)
	if err != nil {
		log.Fatal(err.Error())
	}
	announcements := make(chan Message)
	c.SetHandler(queueConsumer{
		C: announcements,
	})
	c.ConnectToNSQLookupd(s.nsqLookupdHTTPAddr)

	// listen for new announcements
	for {
		msg := <-announcements

		// if the announcement isn't about this contentType we're not interested
		if msg.ContentType != contentType {
			continue
		}

		// if the announcement is about this content type, then we need to associate
		// this colony consumer with a new nsq.Consumer.
		log.Println("connecting to new topic:", msg.Topic.getName())
		c, err := nsq.NewConsumer(msg.Topic.getName(), channel, conf)
		if err != nil {
			log.Fatal(err.Error())
		}
		c.SetHandler(queueConsumer{
			C: inbound,
		})
		c.ConnectToNSQLookupd(s.nsqLookupdHTTPAddr)
	}
}
