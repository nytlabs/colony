package main

import (
	"log"

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
				log.Println("got ant", ant)
				m := s.NewMessage("bees", []byte("bee! bzz bzz"))
				s.Produce(m, func(m colony.Message) error {
					log.Println(m)
					return nil
				},
				)
			}
		}
	}()
	<-quitChan
}
