package main

import (
	"fmt"
	"log"

	"github.com/calvinrzachman/rpc-playground/learnrpc"
)

func main() {

	fmt.Println("Driver method for server")
	quit := make(chan struct{})

	s := learnrpc.NewServer(quit)

	if err := s.Start(); err != nil {
		log.Fatalf("failed to start: %s", err)
	}
}
