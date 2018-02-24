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

type Client struct {
	process    chan []byte
	h          *hub
	addr       *net.UDPAddr
	name       string
	privateKey string
	msgType    string
	rcvFreq    float32
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

const DescriptionTopic = "description"
const LeafletPrefix = "canopy_leaflet_"

// processor from client decompresses the packet and unmarshals the JSON
// to retrieve destination information. It then forwards the original packet
// to the desired client senders.
func (c *Client) processor() {
	lastTime := 0.0
	for {
        msg := <-c.process
		rdr, err := zlib.NewReader(bytes.NewBuffer(msg))
		if err != nil {
            log.WithFields(log.Fields{
                "error": err,
            }).Fatal("Decompress error")
			continue
		}
		var out bytes.Buffer
		io.Copy(&out, rdr)
		rdr.Close()
		decompressedRaw := out.Bytes()
        decompressed := decompressedRaw[4:]
		var m message
		json.Unmarshal(decompressed, &m)

		if _, ok := c.rateLoggers[m.Topic]; !ok {
			c.rateLoggers[m.Topic] = loggers.NewRateLogger(100)
		}

		c.rateLoggers[m.Topic].Log("Client frequency", log.Fields{
			"topic": m.Topic,
			"from":  m.From,
		})

		// Ensure message is coming from correct client.
		if m.From != c.name || m.PrivateKey != c.privateKey {
			continue
		}
		// Ensure messages are sent in order.
		if m.Stamp < lastTime {
			continue
		}
		lastTime = m.Stamp
		c.msgType = m.Type
		splitTopic := strings.Split(m.Topic, "/")
        topicName := splitTopic[len(splitTopic)-1]
        clientName := splitTopic[1]
		if topicName == DescriptionTopic {
			var d description
			json.Unmarshal(m.Msg, &d)
			if sender, ok := c.h.clientMap[c.privateKey][clientName]; ok {
				sender.description = d.Data
			}
		}
		for name := range c.h.clientMap[c.privateKey] {
			if strings.HasPrefix(name, LeafletPrefix) {
				m.To = append(m.To, name)
			}
		}
		list := make([]string, 0)
		for _, to := range m.To {
			if sender, ok := c.h.clientMap[c.privateKey][to]; ok {
				exists := false
				for _, check := range list {
					if check == to {
						exists = true
						break
					}
				}
				if !exists {
					list = append(list, to)
					snd := SendPacket{addr: sender.addr, data: append([]byte{}, msg...)}
					select {
					case c.h.sendChannel <- snd:
					default:
					}
				}
			} else { // Regex
				for name, sender := range c.h.clientMap[c.privateKey] {
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
								snd := SendPacket{addr: sender.addr, data: append([]byte{}, msg...)}
								select {
								case c.h.sendChannel <- snd:
								default:
								}
							}
						}
					}
				}
			}
		}
	}
}
