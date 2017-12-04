// Package main runs the main server comm program.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/websocket"
	"github.com/spf13/viper"
	"go/build"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"text/template"
)

// Page is a graph of nodes and edges in the
// graph visualization tool.
type Page struct {
	Nodes string
	Edges string
}

// Node is a node in the graph visualization tool.
type Node struct {
	ID              string `json:"id"`
	Label           string `json:"label"`
	Mass            int    `json:"mass,omitempty"`
	Shape           string `json:"shape,omitempty"`
	ShapeProperties struct {
		BorderRadius int `json:"borderRadius"`
	} `json:"shapeProperties,omitempty"`
	Title string `json:"title,omitempty"`
	Group string `json:"group,omitempty"`
}

// Edge is an edge in the graph visualization tool.
type Edge struct {
	ID     string `json:"id"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Arrows struct {
		To struct {
			Enabled     bool    `json:"enabled"`
			ScaleFactor float32 `json:"scaleFactor"`
		} `json:"to"`
	} `json:"arrows,omitempty"`
	Font struct {
		Align string `json:"align"`
	} `json:"font,omitempty"`
	Label string `json:"label"`
	Group string `json:"group,omitempty"`
	Color struct {
		Inherit string `json:"inherit"`
	} `json:"color,omitempty"`
}

// hub is a connection point between a network of senders and receivers.
// Communication between senders and receivers is logged to a database
// if a database writer is specified.
type hub struct {
	receivers map[*receiver]bool
	senders   map[*sender]bool
	senderMap map[string]map[string]*sender
	dbw       dbwriter
}

// newHub creates a new hub object.
func newHub() *hub {
	return &hub{
		receivers: make(map[*receiver]bool),
		senders:   make(map[*sender]bool),
		senderMap: make(map[string]map[string]*sender),
	}
}

var upgrader = &websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}

// wsHandler handles HTTP server requests for client connections.
type wsHandler struct {
	h *hub
}

// ServeHTTP from wsHandler responds to connections from clients.
func (wsh wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if (*r).RequestURI[:len("/graph")] == "/graph" || (*r).RequestURI[:len("/favicon")] == "/favicon" {
		return
	}
	// for rcv, _ := range wsh.h.receivers {
	//     if rcv.name == (*r).RequestURI {
	//         log.Printf("Already connected to: %s", (*r).RequestURI)
	//         return
	//     }
	// }
	// for snd, _ := range wsh.h.senders {
	//     if snd.name == (*r).RequestURI {
	//         log.Printf("Already connected to: %s", (*r).RequestURI)
	//         return
	//     }
	// }
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("ConnectionError:", err)
		return
	}
	log.Printf("Connected to: %s", (*r).RequestURI)
	if db != none {
		wsh.h.dbw.setAdd(true, "clients:list", (*r).RequestURI)
	}
	split := strings.Split((*r).RequestURI, "/")
	if _, ok := wsh.h.senderMap[split[1]]; !ok {
		wsh.h.senderMap[split[1]] = make(map[string]*sender)
	}
	if strings.HasSuffix((*r).RequestURI, "receiving") {
		snd := &sender{send: make(chan sendChannel, 2), ws: ws, h: wsh.h,
			name: (*r).RequestURI, privateKey: split[1],
			shortname: split[2], freqs: make(map[*receiver]float32)}
		wsh.h.senders[snd] = true
		wsh.h.senderMap[split[1]][split[2]] = snd
		defer func() {
			delete(wsh.h.senders, snd)
			delete(wsh.h.senderMap[split[1]], split[2])
			close(snd.send)
		}()
		snd.writer()

		if db != none {
			log.Printf("Disconnected from: %s", (*r).RequestURI)
			wsh.h.dbw.setRemove(true, "clients:list", (*r).RequestURI)
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":name")
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":privateKey")
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":description")
			for key := range snd.freqs {
				wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+
					":freq:"+key.name[len("/"+key.privateKey):])
			}
		}
	} else {
		rcv := &receiver{process: make(chan []byte, 2), ws: ws, h: wsh.h,
			name: (*r).RequestURI, privateKey: split[1],
			shortname: split[2]}
		rcv.h.receivers[rcv] = true
		defer func() {
			delete(rcv.h.receivers, rcv)
			close(rcv.process)
		}()
		go rcv.processor()
		rcv.reader()
		log.Printf("Disconnected from: %s", (*r).RequestURI)

		if db != none {
			wsh.h.dbw.setRemove(true, "clients:list", (*r).RequestURI)
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":to")
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":from")
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":topic")
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":type")
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":stamp")
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":msg")
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":privateKey")
			wsh.h.dbw.deleteKey(true, "clients:"+(*r).RequestURI+":freq")
		}
	}
}

// defaultAssetPath creates a default path to assets
func defaultAssetPath() string {
	p, err := build.Default.Import("github.com/canopy-ros/canopy_server_comm", "",
		build.FindOnly)
	if err != nil {
		return "."
	}
	return p.Dir
}

// graphHandler handles HTTP server requests for the
// graph visualization tool.
type graphHandler struct {
	h *hub
}

// ServeHTTP from graphHandler responds to browser requests for
// the visualization tool.
func (wsh graphHandler) ServeHTTP(c http.ResponseWriter, req *http.Request) {
	posturl := strings.Split(req.URL.Path[len("/graph/"):], "/")
	privateKey := posturl[0]
	if len(posturl) > 1 {
		if posturl[1] == "updateedges" {
			edges := make([]Edge, 0)
			for rcv := range wsh.h.receivers {
				split := strings.Split(rcv.name, "/")
				if split[len(split)-1] == "description" {
					continue
				}
				if split[1] == privateKey {
					subname := rcv.name[len("/"+privateKey):]
					edge := Edge{
						ID:    split[2] + " " + subname,
						Label: fmt.Sprintf("%.2f", rcv.rcvFreq) + " Hz",
					}
					edges = append(edges, edge)
					for _, name := range rcv.to {
						edge = Edge{
							ID:    subname + " " + name,
							Label: fmt.Sprintf("%.2f", wsh.h.senderMap[privateKey][name].freqs[rcv]) + " Hz",
						}
						edges = append(edges, edge)
					}
				}
			}
			edgesRes, _ := json.Marshal(edges)
			c.Write([]byte("{\"data\": " + string(edgesRes) + "}"))
			return
		}
	}
	nodes := make([]Node, 0)
	if sMap, ok := wsh.h.senderMap[privateKey]; ok {
		for name, sender := range sMap {
			node := Node{
				ID:    name,
				Label: name,
				Mass:  8,
				Group: name,
				Title: sender.description,
			}
			nodes = append(nodes, node)
		}
	} else {
		return
	}
	edges := make([]Edge, 0)
	for rcv := range wsh.h.receivers {
		split := strings.Split(rcv.name, "/")
		if split[len(split)-1] == "description" {
			continue
		}
		if split[1] == privateKey {
			subname := rcv.name[len("/"+privateKey):]
			node := Node{
				ID:    subname,
				Label: subname,
				Mass:  5,
				Shape: "box",
				ShapeProperties: struct {
					BorderRadius int `json:"borderRadius"`
				}{
					BorderRadius: 3,
				},
				Title: rcv.msgType,
				Group: split[2],
			}
			nodes = append(nodes, node)
			edge := Edge{
				ID:   split[2] + " " + subname,
				From: split[2],
				To:   subname,
				Arrows: struct {
					To struct {
						Enabled     bool    `json:"enabled"`
						ScaleFactor float32 `json:"scaleFactor"`
					} `json:"to"`
				}{
					To: struct {
						Enabled     bool    `json:"enabled"`
						ScaleFactor float32 `json:"scaleFactor"`
					}{
						Enabled:     true,
						ScaleFactor: 0.5,
					},
				},
				Font: struct {
					Align string `json:"align"`
				}{
					Align: "top",
				},
				Label: fmt.Sprintf("%.2f", rcv.rcvFreq) + " Hz",
				Group: split[2],
			}
			edges = append(edges, edge)
			for _, name := range rcv.to {
				edge = Edge{
					ID:   subname + " " + name,
					From: subname,
					To:   name,
					Arrows: struct {
						To struct {
							Enabled     bool    `json:"enabled"`
							ScaleFactor float32 `json:"scaleFactor"`
						} `json:"to"`
					}{
						To: struct {
							Enabled     bool    `json:"enabled"`
							ScaleFactor float32 `json:"scaleFactor"`
						}{
							Enabled:     true,
							ScaleFactor: 0.5,
						},
					},
					Font: struct {
						Align string `json:"align"`
					}{
						Align: "top",
					},
					Label: fmt.Sprintf("%.2f", wsh.h.senderMap[privateKey][name].freqs[rcv]) + " Hz",
					Group: name,
					Color: struct {
						Inherit string `json:"inherit"`
					}{
						Inherit: "to",
					},
				}
				edges = append(edges, edge)
			}
		}
	}
	edgesRes, _ := json.Marshal(edges)
	nodesRes, _ := json.Marshal(nodes)
	if len(posturl) > 1 {
		if posturl[1] == "update" {
			c.Write([]byte("{\"nodes\": " + string(nodesRes) +
				", \"edges\": " + string(edgesRes) + "}"))
			return
		}
	}
	page := Page{Nodes: string(nodesRes), Edges: string(edgesRes)}
	var graphTempl *template.Template
	graphTempl = template.Must(template.ParseFiles(filepath.Join(*assets,
		"graph/graph.html")))
	graphTempl.Execute(c, page)
}

// Options for the 'db' key in the config file
const (
	redis string = "redis"
	none  string = "none"
)

var db string
var addr = flag.String("addr", ":50000", "http service address")
var assets = flag.String("assets", defaultAssetPath(), "path to assets")

func main() {
	log.Println("Canopy communication server started.")

	// get config
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/canopy/")
	viper.SetConfigName("config")
	viper.SetDefault("db", none)
	err := viper.ReadInConfig()
	db := viper.GetString("db")

	flag.Parse()
	fs := http.FileServer(http.Dir("graph/js"))
	http.Handle("/graph/js/", http.StripPrefix("/graph/js/", fs))
	h := newHub()
	http.Handle("/graph/", graphHandler{h: h})

	// initialize database writer
	switch db {
	case redis:
		log.Println("Initializing redis.")
		c, err := redis.Dial("tcp", ":6379")
		if err != nil {
			panic(err)
		}
		dbw := &redisWriter{conn: &c, commChannel: make(chan command, 2)}
		h.dbw = dbw
		c.Do("DEL", "clients:list")
		defer func() {
			c.Do("DEL", "clients:list")
			c.Close()
		}()
		go dbw.writer()
	default:
		log.Println("No database specified.")
	}
	http.Handle("/", wsHandler{h: h})

	if err = http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal("ListenAndServe:", err)
	}
}
