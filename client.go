package main

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io"
	"net"
	"regexp"
	"strings"

	"github.com/canopy-ros/canopy_server_comm/loggers"
	log "github.com/sirupsen/logrus"
)

type client struct {
	process     chan []byte
	h           *hub
	addr        *net.UDPAddr
	name        string
	privateKey  string
	msgType     string
	rcvFreq     float32
	description string
	rateLoggers map[string]*loggers.RateLogger
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

// processor from client decompresses the packet and unmarshals the JSON
// to retrieve destination information. It then forwards the original packet
// to the desired client senders.
func (r *client) processor() {
	lastTime := 0.0
	// for stay, timeout := true, time.After(10 * time.Second); stay; {
	//     select {
	//     case <-r.process:
	//         stay = false
	//     case <-timeout:
	//         stay = false
	//     default:
	//         r.h.sendChannel <- sendPacket{addr: r.addr, data: []byte("HANDSHAKE")}
	//         time.Sleep(500 * time.Millisecond)
	//     }
	// }
	for {
		msg := <-r.process
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

		if _, ok := r.rateLoggers[m.Topic]; !ok {
			r.rateLoggers[m.Topic] = loggers.NewRateLogger(100)
		}

		r.rateLoggers[m.Topic].Log("Client frequency", log.Fields{
			"topic": m.Topic,
			"from":  m.From,
		})
		//log.Println("To:", m.To)
		// Ensure message is coming from correct client.
		if m.From != r.name || m.PrivateKey != r.privateKey {
			continue
		}
		// Ensure messages are sent in order.
		if m.Stamp < lastTime {
			continue
		}
		lastTime = m.Stamp
		r.msgType = m.Type
		split := strings.Split(m.Topic, "/")
		if split[len(split)-1] == "description" {
			var d description
			json.Unmarshal(m.Msg, &d)
			if sender, ok := r.h.clientMap[r.privateKey][split[1]]; ok {
				sender.description = d.Data
			}
		}
		for name := range r.h.clientMap[r.privateKey] {
			if strings.HasPrefix(name, "canopy_leaflet_") {
				m.To = append(m.To, name)
			}
		}
		list := make([]string, 0)
		for _, to := range m.To {
			if sender, ok := r.h.clientMap[r.privateKey][to]; ok {
				exists := false
				for _, check := range list {
					if check == to {
						exists = true
						break
					}
				}
				if !exists {
					list = append(list, to)
					snd := sendPacket{addr: sender.addr, data: append([]byte{}, msg...)}
					select {
					case r.h.sendChannel <- snd:
					default:
					}
				}
			} else { // Regex
				for name, sender := range r.h.clientMap[r.privateKey] {
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
								snd := sendPacket{addr: sender.addr, data: append([]byte{}, msg...)}
								select {
								case r.h.sendChannel <- snd:
								default:
								}
							}
						}
					}
				}
			}
		}
		if db != dbNone {
			r.h.dbw.AddKey(false, "clients:"+r.name+":from", m.From)
			r.h.dbw.AddKey(false, "clients:"+r.name+":topic", m.Topic)
			r.h.dbw.AddKey(false, "clients:"+r.name+":type", m.Type)
			r.h.dbw.AddKey(false, "clients:"+r.name+":stamp", m.Stamp)
			r.h.dbw.AddKey(false, "clients:"+r.name+":msg", m.Msg)
			r.h.dbw.AddKey(false, "clients:"+r.name+":privateKey", m.PrivateKey)
		}
	}
}
