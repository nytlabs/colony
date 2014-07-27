package main

import (
	"flag"
	"github.com/nytlabs/colony"
	"log"
)

var (
	contentType = flag.String("contentType", "", "content type of test message")
	payload     = flag.String("payload", "", "payload of test message")
)

func main() {
	flag.Parse()

	lookupHTTPa := "localhost:4161"
	daemona := "localhost:4150"
	daemonHTTPaddr := "localhost:4151"
	s := colony.NewService("Test", "1", lookupHTTPa, daemona, daemonHTTPaddr)
	log.Println("ct", *contentType)
	s.Announce(*contentType)
	m := s.NewMessage(*contentType, []byte(*payload))
	s.Emit(m)
}
