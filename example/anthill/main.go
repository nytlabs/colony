package main

import (
	"log"
	"time"

	"github.com/nytlabs/colony"
)

func main() {
	lookupHTTPa := "localhost:4161"
	quitChan := make(chan bool)

	log.Println("starting anteater service")
	s := colony.NewService("Anthill", "1", lookupHTTPa)
	s.Announce("ants")

	log.Println("starting ticker")
	ticker := time.NewTicker(time.Duration(5) * time.Second)

	go func() {
		for {
			<-ticker.C
			log.Println("ant!")
			m := s.NewMessage("ants", []byte("ant!"))
			s.Emit(m)
		}
	}()

	<-quitChan

}
