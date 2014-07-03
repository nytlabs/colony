package main

import (
	"log"
	"time"

	"github.com/nytlabs/colony"
)

func responseHandler(c chan colony.Message) error {
	d, _ := time.ParseDuration("500ms")
	timeoutTimer := time.NewTimer(d)
	var m colony.Message
	select {
	case m = <-c:
		log.Println("got response from", m.FromName, "of type", m.ContentType, ":", string(m.Payload))
	case <-timeoutTimer.C:
		log.Println("Timeout! What is honey badger up to do you think?")
		return nil
	}
	log.Println("waiting 2 more seconds just to see if anything else happens")
	timer := time.NewTimer(time.Duration(2) * time.Second)
	select {
	case m = <-c:
		log.Println("got ANOTHER response from", m.FromName, ":", string(m.Payload))
	case <-timer.C:
		log.Println("got nothing, clearly honey badger doesn't give a s**t!")
		// https://www.youtube.com/watch?v=4r7wHMg5Yjg
	}
	return nil
}

func main() {
	lookupa := "localhost:4160"
	lookupHTTPa := "localhost:4161"
	daemona := "localhost:4150"
	daemonHTTPaddr := "localhost:4151"
	quitChan := make(chan bool)

	log.Println("starting anteater service")
	s := colony.NewService("Anteater", "1", lookupa, lookupHTTPa, daemona, daemonHTTPaddr)

	log.Println("starting ant consumer")
	ants := s.NewConsumer("ants")

	go func() {
		for {
			ant := <-ants.C
			log.Println("got ant", string(ant.Payload))
			m := s.NewMessage("bees", []byte("bee! bzz bzz"))

			s.Produce(m, responseHandler)
		}
	}()

	<-quitChan
}
