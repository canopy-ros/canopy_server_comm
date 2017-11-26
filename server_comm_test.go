package main

import (
	"net/http"
	"time"
	"log"
	"testing"
)

// Test communication server
func TestServerComm(t *testing.T) {
        r := &wsHandler{}
        srv := &http.Server {
		Addr: ":8000",
		Handler: r,
	}

	ticker := time.NewTicker(time.Second)
	go func() {
		for t := range ticker.C {
			log.Println("Tick at", t)
		}
	}()

	go srv.ListenAndServe()
	log.Println("Server is running");
	time.Sleep(time.Second * 5)
	ticker.Stop()
	srv.Close()
	log.Println("Server has been closed");
}
