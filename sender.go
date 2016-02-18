package main

import (
	"github.com/gorilla/websocket"
	"log"
	"time"
)

type sendChannel struct {
	r *receiver
	data []byte
}

type sender struct {
	ws *websocket.Conn
	send chan sendChannel
	name string
	private_key string
	description string
}

func (s *sender) heartbeat(beat chan bool, stop chan bool) {
	for {
		exit := false
		select {
		case <-beat:
		case <-time.After(500 * time.Millisecond):
			message := make([]byte, 1)
			s.send <- sendChannel{r: &receiver{}, data: message}
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
	count := make(map[*receiver]int32)
	last_time := make(map[*receiver]time.Time)
	for message := range s.send {
		select {
		case hbeat <- true:
		default:
		}
		err := s.ws.WriteMessage(websocket.BinaryMessage, message.data)
		if err != nil {
			log.Printf("[%s] WriteError: %s", s.name, err)
			break
		}
		if _, ok := count[message.r]; !ok {
			count[message.r] = 0
			last_time[message.r] = time.Now()
		}
		count[message.r] += 1
		if count[message.r] >= 20 {
			message.r.sendFreq[s] = 20.0 * 1e9 / float32((time.Now().Sub(last_time[message.r])))
			last_time[message.r] = time.Now()
			count[message.r] = 0
                }

	}
	stop <- true
	s.ws.Close()
}
