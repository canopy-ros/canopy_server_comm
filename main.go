package main

import (
	"github.com/gorilla/websocket"
	"net/http"
	"flag"
	"log"
	"strings"
	"go/build"
	"text/template"
	"path/filepath"
)

type Page struct {
	Nodes string
	Edges string
}

type hub struct {
	receivers map[*receiver]bool
	senders map[*sender]bool
	senderMap map[string]map[string]*sender
	topicReceivers map[*receiver][]string
}

func newHub() *hub {
	return &hub{
		receivers: make(map[*receiver]bool),
		senders: make(map[*sender]bool),
		senderMap: make(map[string]map[string]*sender),
		topicReceivers: make(map[*receiver][]string),
	}
}

var upgrader = &websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}

type wsHandler struct {
	h *hub
}

func (wsh wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if (*r).RequestURI[:len("/graph")] == "/graph" {
		return
	}
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

func defaultAssetPath() string {
	p, err := build.Default.Import("github.com/roscloud/roscloud_server", "", build.FindOnly)
	if err != nil {
		return "."
	}
	return p.Dir
}

type graphHandler struct {
        h *hub
}

func (wsh graphHandler) ServeHTTP(c http.ResponseWriter, req *http.Request) {
	private_key := req.URL.Path[len("/graph/"):]
	nodes := ""
	if sMap, ok := wsh.h.senderMap[private_key]; ok {
		for name, _ := range sMap {
			nodes += "{id: '" + name + "', label: '" + name + "', mass: 10, value: 5},"
		}
	}
	edges := ""
	for rcv, _ := range wsh.h.receivers {
		split := strings.Split(rcv.name, "/")
		if split[1] == private_key {
			for _, name := range wsh.h.topicReceivers[rcv] {
				edges += "{from: '" + split[2] + "', to: '" + name + "', arrows: 'to', font: {align: 'top'}, label: '" + rcv.name[len("/" + private_key):] + "'},"
			}
		}
	}
	page := Page{Nodes: nodes, Edges: edges}
	var graphTempl *template.Template
        graphTempl = template.Must(template.ParseFiles(filepath.Join(*assets, "graph/graph.html")))
	graphTempl.Execute(c, page)
}

var addr = flag.String("addr", ":50000", "http service address")
var assets = flag.String("assets", defaultAssetPath(), "path to assets")

func main() {
	log.Println("ROSCloud server started.")
	flag.Parse()
	fs := http.FileServer(http.Dir("graph/vis"))
	http.Handle("/graph/vis/", http.StripPrefix("/graph/vis/", fs))
	h := newHub()
	http.Handle("/graph/", graphHandler{h: h})
	http.Handle("/", wsHandler{h: h})
	if err := http.ListenAndServe(*addr, nil); err != nil {
        	log.Fatal("ListenAndServe:", err)
    	}
}
