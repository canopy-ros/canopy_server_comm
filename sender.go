package main

import (
	"github.com/gorilla/websocket"
	"log"
)

type sender struct {
	ws *websocket.Conn
	send chan []byte
	name string
}

func (s *sender) writer() {
	for message := range s.send {
	err := s.ws.WriteMessage(websocket.BinaryMessage, message)
		if err != nil {
			log.Println("WriteError:", err)
			break
		}
	}
	s.ws.Close()
}
