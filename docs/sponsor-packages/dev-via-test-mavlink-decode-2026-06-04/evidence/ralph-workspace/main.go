package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	receiver := NewReceiver()
	server := NewServer(receiver)
	server.RegisterHandlers()

	// Start listening for UDP on port 14540 in a goroutine.
	go func() {
		if err := receiver.ListenUDP(14540); err != nil {
			log.Printf("UDP listen error: %v", err)
		}
	}()

	// Start HTTP server on port 8080.
	addr := ":8080"
	fmt.Printf("Starting HTTP server on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}
