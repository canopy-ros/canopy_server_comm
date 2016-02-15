package main

import (
	"github.com/gorilla/websocket"
	"net/http"
	"flag"
	"log"
	"strings"
)

type hub struct {
	receivers map[*receiver]bool
	senders map[*sender]bool
	senderMap map[string]*sender
}

func newHub() *hub {
	return &hub{
		receivers: make(map[*receiver]bool),
		senders: make(map[*sender]bool),
		senderMap: make(map[string]*sender),
	}
}

var upgrader = &websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}

type wsHandler struct {
    h *hub
}

func (wsh wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ConnectionError:", err)
		return
	}
	log.Printf("Connected to: %s", (*r).RequestURI)
	if strings.HasSuffix((*r).RequestURI, "receiving") {
		snd := &sender{send: make(chan []byte, 2), ws: ws, name: (*r).RequestURI}
		split := strings.Split((*r).RequestURI, "/")
		wsh.h.senders[snd] = true
		wsh.h.senderMap[split[1]] = snd
		defer func() {
			delete(wsh.h.senders, snd)
			delete(wsh.h.senderMap, split[1])
			close(snd.send)
		}()
		snd.writer()
	} else {
		rcv := &receiver{process: make(chan []byte, 2), ws: ws, h: wsh.h, name: (*r).RequestURI}
		rcv.h.receivers[rcv] = true
		defer func() {
			delete(rcv.h.receivers, rcv)
			close(rcv.process)
		}()
		go rcv.processor()
		rcv.reader()
	}
}

var addr = flag.String("addr", "localhost:9000", "http service address")

func main() {
	flag.Parse()
	h := newHub()
	http.Handle("/", wsHandler{h: h})
	if err := http.ListenAndServe(*addr, nil); err != nil {
        	log.Fatal("ListenAndServe:", err)
    	}
}
