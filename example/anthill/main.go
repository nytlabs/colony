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
	s := colony.NewService("Anthill", "1", lookupa, lookupHTTPa, daemona, daemonHTTPaddr)

	log.Println("starting ticker")
	ticker := time.NewTicker(time.Duration(5) * time.Second)

	go func() {
		for {
			<-ticker.C
			log.Println("ant!")
			m := s.NewMessage("ants", []byte("ant!"))
			s.Produce(m, nil)
		}
	}()

	<-quitChan

}
