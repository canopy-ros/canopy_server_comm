package main

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"regexp"
	"strings"
	"time"
)

type receiver struct {
	ws         *websocket.Conn
	process    chan []byte
	h          *hub
	name       string
	shortname  string
	privateKey string
	to         []string
	msgType    string
	rcvFreq    float32
}

type message struct {
	To         []string
	From       string
	Topic      string
	Type       string
	Stamp      float64
	Msg        json.RawMessage
	PrivateKey string
}

type description struct {
	Data string
}

// processor from receiver decompresses the packet and unmarshals the JSON
// to retrieve destination information. It then forwards the original packet
// to the desired client senders.
func (r *receiver) processor() {
	lastTime := 0.0
	for {
		msg := <-r.process
		snd := sendChannel{r: r, data: msg}
		rdr, err := zlib.NewReader(bytes.NewBuffer(msg))
		if err != nil {
			break
		}
		var out bytes.Buffer
		io.Copy(&out, rdr)
		rdr.Close()
		decompressed := out.Bytes()
		var m message
		json.Unmarshal(decompressed[4:], &m)
		//log.Println("To:", m.To)
		// Ensure message is coming from correct client.
		if m.From != r.shortname || m.PrivateKey != r.privateKey {
			return
		}
		// Ensure messages are sent in order.
		if m.Stamp < lastTime {
			return
		}
		lastTime = m.Stamp
		r.msgType = m.Type
		split := strings.Split(m.Topic, "/")
		if split[len(split)-1] == "description" {
			var d description
			json.Unmarshal(m.Msg, &d)
			if sender, ok := r.h.senderMap[r.privateKey][split[1]]; ok {
				sender.description = d.Data
			}
		}
		for name := range r.h.senderMap[r.privateKey] {
			if strings.HasPrefix(name, "canopy_leaflet_") {
				m.To = append(m.To, name)
			}
		}
		list := make([]string, 0)
		for _, to := range m.To {
			if sender, ok := r.h.senderMap[r.privateKey][to]; ok {
				exists := false
				for _, check := range list {
					if check == to {
						exists = true
						break
					}
				}
				if !exists {
					list = append(list, to)
					select {
					case sender.send <- snd:
					default:
					}
				}
			} else { // Regex
				for name, sender := range r.h.senderMap[r.privateKey] {
					if name != m.From {
						match, _ := regexp.MatchString(to, name)
						if match {
							exists := false
							for _, check := range list {
								if check == name {
									exists = true
									break
								}
							}
							if !exists {
								list = append(list, name)
								select {
								case sender.send <- snd:
								default:
								}
							}
						}
					}
				}
			}
		}
		r.to = list
		if db != none {
			r.h.dbw.addKey(false, "clients:"+r.name+":to", strings.Join(r.to, " "))
			r.h.dbw.addKey(false, "clients:"+r.name+":from", m.From)
			r.h.dbw.addKey(false, "clients:"+r.name+":topic", m.Topic)
			r.h.dbw.addKey(false, "clients:"+r.name+":type", m.Type)
			r.h.dbw.addKey(false, "clients:"+r.name+":stamp", m.Stamp)
			r.h.dbw.addKey(false, "clients:"+r.name+":msg", m.Msg)
			r.h.dbw.addKey(false, "clients:"+r.name+":privateKey", m.PrivateKey)
		}
	}
}

// reader from receiver continually polls the socket for new packets,
// and then sends them to be processed. It also calculates read frequencies.
func (r *receiver) reader() {
	count := 0
	lastTime := time.Now()
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
		count++
		if count == 20 {
			r.rcvFreq = 20.0 * 1e9 / float32((time.Now().Sub(lastTime)))
			lastTime = time.Now()
			count = 0
		}

		if db != none {
			r.h.dbw.addKey(false, "clients:"+r.name+":freq", r.rcvFreq)
		}
	}
	r.ws.Close()
}
