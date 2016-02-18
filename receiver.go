package main

import (
	"github.com/gorilla/websocket"
	"log"
	"bytes"
	"io"
	"compress/zlib"
	"encoding/json"
	"time"
)

type receiver struct {
	ws *websocket.Conn
	process chan []byte
	h *hub
	name string
	private_key string
	to []string
	msg_type string
	rcvFreq float32
	sendFreq map[*sender]float32
}

type message struct {
	To []string
	From string
	Topic string
	Type string
	Stamp float64
}

func (r *receiver) processor() {
	for {
		msg := <- r.process
		snd := sendChannel{r: r, data: msg}
		rdr, err := zlib.NewReader(bytes.NewBuffer(msg))
		if err != nil {
			break;
		}
		var out bytes.Buffer
		io.Copy(&out, rdr)
		rdr.Close()
		decompressed := out.Bytes()
		var m message
		json.Unmarshal(decompressed[4:], &m)
		//log.Println("To:", m.To)
		r.msg_type = m.Type
		for _, to := range m.To {
			list := make([]string, 0)
			if to == "*" {
				for name, sender := range r.h.senderMap[r.private_key] {
					if name != m.From {
						list = append(list, name)
						select {
						case sender.send <- snd:
						default:
						}
					}
				}
			} else if sender, ok := r.h.senderMap[r.private_key][to]; ok {
				list = append(list, to)
				select {
				case sender.send <- snd:
				default:
				}
			}
			r.to = list
		}
	}
}

func (r *receiver) reader() {
	count := 0
	last_time := time.Now()
	for {
		_, message, err := r.ws.ReadMessage()
		if err != nil {
    			log.Printf("[%s] ReadError: %s", r.name, err)
			break
		}
		select {
		case r.process <- message:
		default:
		}
		//log.Printf("%s: %d", r.name, unsafe.Sizeof(message))
		msg := make([]byte, 1)
		err = r.ws.WriteMessage(websocket.BinaryMessage, msg)
		if err != nil {
			break
		}
		count += 1
		if count == 20 {
			r.rcvFreq = 20.0 * 1e9 / float32((time.Now().Sub(last_time)))
			last_time = time.Now()
			count = 0
		}
	}
	r.ws.Close()
}
