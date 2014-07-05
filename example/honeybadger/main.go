package main

import (
	"log"
	"math/rand"

	"github.com/nytlabs/colony"
)

func main() {
	lookupHTTPa := "localhost:4161"
	daemona := "localhost:4150"
	daemonHTTPaddr := "localhost:4151"
	quitChan := make(chan bool)
	s := colony.NewService("honeybadger", "1", lookupHTTPa, daemona, daemonHTTPaddr)

	go s.Consume("bees", func(bees <-chan colony.Message) error {
		for {
			bee := <-bees
			log.Println("got bee", string(bee.Payload), "!")
			m := s.NewResponse(bee, "HoneyBadgerEtiquette", []byte("thanks for the bee!"))
			s.Produce(m, nil)
			log.Println("sent response")
			if rand.Float64() < 0.5 {
				m = s.NewResponse(bee, "SnakeRequest", []byte("got any snkaes?"))
				s.Produce(m, nil)
			}
		}
		return nil
	},
	)

	<-quitChan
}
