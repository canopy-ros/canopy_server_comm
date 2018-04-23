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

// Client defines a client connection object
type Client struct {
    process    chan []byte
    hub        *Hub
    addr       *net.UDPAddr
    name       string
    privateKey string
    msgType    string
    rcvFreq    float32
    description string
    rateLoggers map[string]*loggers.RateLogger
}

// Message represents a ROS message from a client
type Message struct {
    To         []string
    From       string
    Topic      string
    Type       string
    Stamp      float64
    Msg        json.RawMessage
    PrivateKey string
}

// Description defines a struct for a description message from a client
type Description struct {
    Data string
}

// DescriptionTopic is the topic name of the client description
const DescriptionTopic = "description"
// LeafletPrefix is the prefix of leaflet client names
const LeafletPrefix = "canopy_leaflet_"
// DecompressedHeaderIndex is the start index of the decompressed packet
const DecompressedHeaderIndex = 4

// Processor from client decompresses the packet and unmarshals the JSON
// to retrieve destination information. It then forwards the original packet
// to the desired clients.
func (c *Client) Processor() {
    lastTime := 0.0
    for {
        packet := <-c.process
        msg, err := c.UnpackMessage(&packet)
        if err != nil {
            continue
        }

        if _, ok := c.rateLoggers[msg.Topic]; !ok {
            c.rateLoggers[msg.Topic] = loggers.NewRateLogger(RateLoggerRate)
        }

        c.rateLoggers[msg.Topic].Log("Client frequency", log.Fields{
            "topic": msg.Topic,
            "from":  msg.From,
        })

        // Ensure message is coming from correct client.
        if msg.From != c.name || msg.PrivateKey != c.privateKey {
            continue
        }
        // Ensure messages are sent in order.
        if msg.Stamp < lastTime {
            continue
        }
        lastTime = msg.Stamp
        c.msgType = msg.Type
        topicName, clientName := processTopicString(msg.Topic)
        if topicName == DescriptionTopic {
            err = c.ProcessDescription(&msg, clientName)
            if err != nil {
                continue
            }
        }
        for name := range c.hub.clientMap[c.privateKey] {
            if strings.HasPrefix(name, LeafletPrefix) {
                msg.To = append(msg.To, name)
            }
        }
        c.SendMessage(&msg, &packet)
    }
}

// UnpackMessage converts raw packet data into a Message struct
func (c *Client) UnpackMessage(packet *[]byte) (Message, error) {
    rdr, err := zlib.NewReader(bytes.NewBuffer(*packet))
    if err != nil {
        log.WithFields(log.Fields{
            "error": err,
        }).Fatal("Decompress error")
        return Message{}, err
    }
    var out bytes.Buffer
    io.Copy(&out, rdr)
    rdr.Close()
    decompressedRaw := out.Bytes()
    decompressed := decompressedRaw[DecompressedHeaderIndex:]
    var m Message
    err = json.Unmarshal(decompressed, &m)
    if err != nil {
        log.WithFields(log.Fields{
            "error": err,
        }).Fatal("Unmarshal error")
        return Message{}, err
    }
    return m, nil
}

// SendMessage sends a packet to another client based on destination data
func (c *Client) SendMessage(m *Message, packet *[]byte) {
    sent := make([]string, 0)
    for _, to := range m.To {
        if sender, ok := c.hub.clientMap[c.privateKey][to]; ok {
            exists := false
            for _, check := range sent {
                if check == to {
                    exists = true
                    break
                }
            }
            if !exists {
                sent = append(sent, to)
                snd := SendPacket{addr: sender.addr, data: append([]byte{}, *packet...)}
                select {
                case c.hub.sendChannel <- snd:
                default:
                }
            }
        } else { // Regex
            for name, sender := range c.hub.clientMap[c.privateKey] {
                if name != m.From {
                    match, _ := regexp.MatchString(to, name)
                    if match {
                        exists := false
                        for _, check := range sent {
                            if check == name {
                                exists = true
                                break
                            }
                        }
                        if !exists {
                            sent = append(sent, name)
                            snd := SendPacket{addr: sender.addr, data: append([]byte{}, *packet...)}
                            select {
                            case c.hub.sendChannel <- snd:
                            default:
                            }
                        }
                    }
                }
            }
        }
    }
}

// ProcessDescription processes a description message and updates client descriptions
func (c *Client) ProcessDescription(m *Message, clientName string) error {
    var desc Description
    err := json.Unmarshal(m.Msg, &desc)
    if err != nil {
        log.WithFields(log.Fields{
            "error": err,
        }).Fatal("Unmarshal error")
        return err
    }
    if sender, ok := c.hub.clientMap[c.privateKey][clientName]; ok {
        sender.description = desc.Data
    }
    return nil
}

func processTopicString(topic string) (string, string) {
    splitTopic := strings.Split(topic, "/")
    topicName := splitTopic[len(splitTopic)-1]
    clientName := splitTopic[1]
    return topicName, clientName
}
