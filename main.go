// Package main runs the main server comm program.
package main

import (
    "flag"
    "github.com/garyburd/redigo/redis"
    "github.com/spf13/viper"
    "log"
    "net"
    "strings"
)

type sendPacket struct {
    addr *net.UDPAddr
    data []byte
}

// hub is a connection point between a network of senders and receivers.
// Communication between senders and receivers is logged to a database
// if a database writer is specified.
type hub struct {
    clientMap map[string]map[string]*client
    sendChannel chan sendPacket
    dbw       DBWriter
}

// newHub creates a new hub object.
func newHub() *hub {
    return &hub{
        clientMap: make(map[string]map[string]*client),
    }
}

// ServeHTTP from wsHandler responds to connections from clients.
func (wsh wsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if len((*r).RequestURI) >= len("/favicon") && (*r).RequestURI[:len("/favicon")] == "/favicon" {
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
    if db != dbNone {
        wsh.h.dbw.SetAdd(true, "clients:list", (*r).RequestURI)
    }
    split := strings.Split((*r).RequestURI, "/")
    if _, ok := wsh.h.senderMap[split[1]]; !ok {
        wsh.h.senderMap[split[1]] = make(map[string]*sender)
    }
    if strings.HasSuffix((*r).RequestURI, "receiving") {
        snd := &sender{send: make(chan sendChannel, 1), ws: ws, h: wsh.h,
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

        if db != dbNone {
            log.Printf("Disconnected from: %s", (*r).RequestURI)
            wsh.h.dbw.SetRemove(true, "clients:list", (*r).RequestURI)
            wsh.h.dbw.DeleteKey(true, "clients:"+(*r).RequestURI+":name",
                "clients:"+(*r).RequestURI+":privateKey",
                "clients:"+(*r).RequestURI+":description")
            for key := range snd.freqs {
                wsh.h.dbw.DeleteKey(true, "clients:"+(*r).RequestURI+
                    ":freq:"+key.name[len("/"+key.privateKey):])
            }
        }
    } else {
        rcv := &receiver{process: make(chan []byte, 1), ws: ws, h: wsh.h,
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

        if db != dbNone {
            wsh.h.dbw.SetRemove(true, "clients:list", (*r).RequestURI)
            wsh.h.dbw.DeleteKey(true, "clients:"+(*r).RequestURI+":to",
                "clients:"+(*r).RequestURI+":from",
                "clients:"+(*r).RequestURI+":topic",
                "clients:"+(*r).RequestURI+":type",
                "clients:"+(*r).RequestURI+":stamp",
                "clients:"+(*r).RequestURI+":msg",
                "clients:"+(*r).RequestURI+":privateKey",
                "clients:"+(*r).RequestURI+":freq")
        }
    }
}


func udpServer(address string, h *hub) {
    serverAddr, err := net.ResolveUDPAddr("udp", addr)
    if err != nil {
        panic(err)
    }
    serverConn, err := net.ListenUDP("udp", serverAddr)
    if err != nil {
        panic(err)
    }
    defer serverConn.Close()

    h.sendChannel = make(chan sendPacket, 5)
    addrMap := make(map[string]*client)
    go sender(serverConn, h.sendChannel)
    buf := make([]byte, 65507)

    for {
        n, addr, err := serverConn.ReadFromUDP(buf)
        if err != nil {
            fmt.Println("Error: ", err)
        }
        if strings.HasPrefix(string(buf[:n]), "CONNECT") {
            split := strings.Split(buf[:n], ":")
            privateKey = split[1]
            name = split[2]
            if _, ok := clientMap[privateKey]; !ok {
                h.clientMap[privateKey] = make(map[string]client)
            }
            if _, ok := clientMap[privateKey][name]; !ok {
                cli := &client{addr: addr, process: make(chan []byte, 1),
                    name: name, privateKey: privateKey}
                h.clientMap[privateKey][name] = cli
                addrMap[addr.IP + ":" + string(addr.Port)] = cli
                go cli.processor()
            }
        } else {
            select {
            case addrMap[addr.IP + ":" + string(addr.Port)].process <- buf[:n]:
            default:
            }
        }
    }
}

func sender(c *net.UDPConn, s chan sendPacket) {
    for message := range s {
        c.WriteToUDP(s.data, s.addr)
    }
}


// Options for the 'db' key in the config file
const (
    dbRedis string = "redis"
    dbNone  string = "none"
)

var db string
var addr = flag.String("addr", ":8080", "http service address")

func main() {
    log.Println("Canopy communication server started.")

    // get config
    viper.AddConfigPath(".")
    viper.AddConfigPath("/etc/canopy/")
    viper.SetConfigName("config")
    viper.SetDefault("db", dbNone)
    err := viper.ReadInConfig()
    db = viper.GetString("db")

    h := newHub()

    // initialize database writer
    switch db {
    case dbRedis:
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
        go dbw.Writer()
    default:
        log.Println("No database specified.")
    }
    udpServer(addr, h)
}
