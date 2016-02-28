package main

import (
    "github.com/gorilla/websocket"
    "log"
    "time"
    "github.com/garyburd/redigo/redis"
)

type sendChannel struct {
    r *receiver
    data []byte
}

type sender struct {
    ws *websocket.Conn
    h *hub
    redisconn *redis.Conn
    send chan sendChannel
    name string
    shortname string
    private_key string
    description string
    freqs map[*receiver]float32
}

// heartbeat from sender sends a heartbeat packet to the client every second,
// when no regular packets are being sent.
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

// writer from sender writes packets to the receiving client.
// It also calculates write frequencies.
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
            s.freqs[message.r] = 20.0 * 1e9 / float32(
                (time.Now().Sub(last_time[message.r])))
            last_time[message.r] = time.Now()
            count[message.r] = 0
        }
        s.h.dbw.write(false, "HMSET", "clients:" + s.name, "private_key", s.private_key,
            "description", s.description, "name", s.shortname)
        for key, value := range s.freqs {
            s.h.dbw.write(false, "HSET", "clients:" + s.name, "freq:" + key.name,
                value)
        }
    }
    stop <- true
    s.ws.Close()
}
