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
	senderMap map[string]map[string]*sender
}

func newHub() *hub {
	return &hub{
		receivers: make(map[*receiver]bool),
		senders: make(map[*sender]bool),
		senderMap: make(map[string]map[string]*sender),
	}
}

var upgrader = &websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}

type wsHandler struct {
    h *hub
}

func (wsh wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for rcv, _ := range wsh.h.receivers {
		if rcv.name == (*r).RequestURI {
			log.Printf("Already connected to: %s", (*r).RequestURI)
			return
		}
	}
	for snd, _ := range wsh.h.senders {
		if snd.name == (*r).RequestURI {
			log.Printf("Already connected to: %s", (*r).RequestURI)
			return
		}
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ConnectionError:", err)
		return
	}
	log.Printf("Connected to: %s", (*r).RequestURI)
	split := strings.Split((*r).RequestURI, "/")
	if _, ok := wsh.h.senderMap[split[1]]; !ok {
		wsh.h.senderMap[split[1]] = make(map[string]*sender)
	}
	if strings.HasSuffix((*r).RequestURI, "receiving") {
		snd := &sender{send: make(chan []byte, 2), ws: ws, name: (*r).RequestURI, private_key: split[1]}
		wsh.h.senders[snd] = true
		wsh.h.senderMap[split[1]][split[2]] = snd
		defer func() {
			delete(wsh.h.senders, snd)
			delete(wsh.h.senderMap[split[1]], split[2])
			close(snd.send)
		}()
		snd.writer()
		log.Printf("Disconnected from: %s", (*r).RequestURI)
	} else {
		rcv := &receiver{process: make(chan []byte, 2), ws: ws, h: wsh.h, name: (*r).RequestURI, private_key: split[1]}
		rcv.h.receivers[rcv] = true
		defer func() {
			delete(rcv.h.receivers, rcv)
			close(rcv.process)
		}()
		go rcv.processor()
		rcv.reader()
		log.Printf("Disconnected from: %s", (*r).RequestURI)
	}
}

var addr = flag.String("addr", ":50000", "http service address")

func main() {
	log.Println("ROSCloud server started.")
	flag.Parse()
	h := newHub()
	http.Handle("/", wsHandler{h: h})
	if err := http.ListenAndServe(*addr, nil); err != nil {
        	log.Fatal("ListenAndServe:", err)
    	}
}
