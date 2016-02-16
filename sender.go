package main

import (
	"github.com/gorilla/websocket"
	"log"
	"time"
)

type sender struct {
	ws *websocket.Conn
	send chan []byte
	name string
	private_key string
}

func (s *sender) heartbeat(beat chan bool, stop chan bool) {
	for {
		exit := false
		select {
		case <-beat:
		case <-time.After(500 * time.Millisecond):
			message := make([]byte, 1)
			s.send <- message
		case <-stop:
			exit = true
		}
		if exit {
			break
		}
	}
}

func (s *sender) writer() {
	hbeat := make(chan bool)
	stop := make(chan bool)
	go s.heartbeat(hbeat, stop)
	for message := range s.send {
		select {
		case hbeat <- true:
		default:
		}
		err := s.ws.WriteMessage(websocket.BinaryMessage, message)
		if err != nil {
			log.Printf("[%s] WriteError: %s", s.name, err)
			break
		}
	}
	stop <- true
	s.ws.Close()
}
