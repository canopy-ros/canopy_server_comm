package main

import (
    "github.com/gorilla/websocket"
    "log"
    "bytes"
    "io"
    "compress/zlib"
    "encoding/json"
    "time"
    "strings"
    "regexp"
    "fmt"
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
    Msg json.RawMessage
}

type description struct {
    Data string
}

// processor from receiver decompresses the packet and unmarshals the JSON
// to retrieve destination information. It then forwards the original packet
// to the desired client senders.
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
        split := strings.Split(m.Topic, "/")
        if (split[len(split) - 1] == "description") {
            var d description
            json.Unmarshal(m.Msg, &d)
            if sender, ok := r.h.senderMap[r.private_key][split[1]]; ok {
                sender.description = d.Data
            }
        }
        for _, to := range m.To {
            list := make([]string, 0)
            if sender, ok := r.h.senderMap[r.private_key][to]; ok {
                list = append(list, to)
                select {
                case sender.send <- snd:
                default:
                }
            } else { // Regex
                for name, sender := range r.h.senderMap[r.private_key] {
                    if name != m.From {
                        match, _ := regexp.MatchString(to, name)
                        if match {
                            list = append(list, name)
                            select {
                            case sender.send <- snd:
                            default:
                            }
                        }
                    }
                }
            }
            r.to = list
        }
        if err != nil {
            fmt.Print(err)
            return
        }
    }
}

// reader from receiver continually polls the socket for new packets,
// and then sends them to be processed. It also calculates read frequencies.
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
