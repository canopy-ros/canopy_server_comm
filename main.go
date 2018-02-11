// Package main runs the main server comm program.
package main

import (
    "flag"
    "github.com/garyburd/redigo/redis"
    "github.com/spf13/viper"
    "log"
    "net"
    "strings"
    "fmt"
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

func udpServer(address string, h *hub) {
    serverAddr, err := net.ResolveUDPAddr("udp", address)
    if err != nil {
        panic(err)
    }
    serverConn, err := net.ListenUDP("udp", serverAddr)
    if err != nil {
        panic(err)
    }
    serverConn.SetReadBuffer(65535)
    serverConn.SetWriteBuffer(65535)
    defer serverConn.Close()

    h.sendChannel = make(chan sendPacket, 5)
    addrMap := make(map[string]*client)
    go sender(serverConn, h.sendChannel)
    buf := make([]byte, 65507)

    for {
        n, addr, err := serverConn.ReadFromUDP(buf)
        if err != nil {
            fmt.Println("Error: UDP read: %v", err)
        }
        if strings.HasPrefix(string(buf[:n]), "CONNECT") {
            split := strings.Split(string(buf[:n]), ":")
            privateKey := split[1]
            name := split[2]
            if _, ok := h.clientMap[privateKey]; !ok {
                h.clientMap[privateKey] = make(map[string]*client)
            }
            if _, ok := h.clientMap[privateKey][name]; !ok {
                log.Println("New client:", privateKey + ":" + name)
                cli := &client{addr: addr, process: make(chan []byte, 1),
                name: name, privateKey: privateKey, h: h}
                h.clientMap[privateKey][name] = cli
                addrMap[string(addr.IP) + ":" + string(addr.Port)] = cli
                if db != dbNone {
                    h.dbw.SetAdd(true, "clients:list", name)
                }
                go cli.processor()
            } else {
                h.clientMap[privateKey][name].addr = addr
                addrMap[string(addr.IP) + ":" + string(addr.Port)] = h.clientMap[privateKey][name]
            }
        } else {
            if _, ok := addrMap[string(addr.IP) + ":" + string(addr.Port)]; ok {
                select {
                case addrMap[string(addr.IP) + ":" + string(addr.Port)].process <- append([]byte{}, buf[:n]...):
                default:
                }
            }
        }
    }
}

func sender(c *net.UDPConn, s <-chan sendPacket) {
    for message := range s {
        _, err := c.WriteToUDP(message.data, message.addr)
        if err != nil {
            log.Printf("Error: UDP write: %v", err)
        }
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
    viper.ReadInConfig()
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
    udpServer(*addr, h)
}
