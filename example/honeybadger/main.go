package main

import (
	"log"
	"math/rand"

	"github.com/nytlabs/colony"
)

func main() {
	lookupa := "localhost:4160"
	lookupHTTPa := "localhost:4161"
	daemona := "localhost:4150"
	daemonHTTPaddr := "localhost:4151"
	quitChan := make(chan bool)
	s := colony.NewService("honeybadger", "1", lookupa, lookupHTTPa, daemona, daemonHTTPaddr)

	bees := s.NewConsumer("bees")

	go func() {
		for {
			select {
			case bee := <-bees.C:
				log.Println("got bee", bee, "!")
				m := s.NewResponse(bee, "Bees", []byte("thanks for the bee!"))
				s.Produce(m, nil)
				if rand.Float64() < 0.5 {
					m = s.NewResponse(bee, "Bees", []byte("got any snkaes?"))
					s.Produce(m, nil)
				}
			}
		}
	}()
	<-quitChan
}
