package main

import (
	"log"
	"time"

	"github.com/nytlabs/colony"
)

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
			select {
			case ant := <-ants.C:
				log.Println("got ant", string(ant.Payload))
				m := s.NewMessage("bees", []byte("bee! bzz bzz"))
				s.Produce(m, func(c chan colony.Message) error {
					m := <-c
					log.Println("got response from", m.FromName, ":", string(m.Payload))
					log.Println("waiting 2 more seconds just to see if anything better comes along")
					ticker := time.NewTicker(time.Duration(2) * time.Second)
					select {
					case m := <-c:
						log.Println("got ANOTHER response from", m.FromName, ":", string(m.Payload))
					case <-ticker.C:
						log.Println("got nothing, clearly", m.FromName, "doesn't give a s**t!")
						// https://www.youtube.com/watch?v=4r7wHMg5Yjg
					}
					return nil
				},
				)
			}
		}
	}()
	<-quitChan
}
