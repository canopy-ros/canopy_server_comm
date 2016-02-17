package main

import (
	"github.com/gorilla/websocket"
	"log"
	"bytes"
	"io"
	"compress/zlib"
	"encoding/json"
)

type receiver struct {
	ws *websocket.Conn
	process chan []byte
	h *hub
	name string
	private_key string
	to []string
	msg_type string
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
						case sender.send <- msg:
						default:
						}
					}
				}
			} else if sender, ok := r.h.senderMap[r.private_key][to]; ok {
				list = append(list, to)
				select {
				case sender.send <- msg:
				default:
				}
			}
			r.to = list
		}
	}
}

func (r *receiver) reader() {
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
	}
	r.ws.Close()
}
