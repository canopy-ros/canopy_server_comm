// Package main runs the main server comm program.
package main

import (
    "flag"
    "net"
    "strings"

    "github.com/canopy-ros/canopy_server_comm/loggers"
    "github.com/garyburd/redigo/redis"
    log "github.com/sirupsen/logrus"
    "github.com/spf13/viper"
)

// SendPacket defines a packet to be sent
type SendPacket struct {
    addr *net.UDPAddr
    data []byte
}

// Hub is a connection point between a network of senders and receivers.
// Communication between senders and receivers is logged to a database
// if a database writer is specified.
type Hub struct {
    clientMap   map[string]map[string]*Client
    sendChannel chan SendPacket
    dbw         DBWriter
}

// NewHub creates a new Hub object.
func NewHub() *Hub {
    return &Hub{
        clientMap: make(map[string]map[string]*Client),
    }
}

// ConnectionBufferSize sets the buffer size for packets
const ConnectionBufferSize = 65535

// RateLoggerRate defines the rate to log rates at
const RateLoggerRate = 100

// UdpServer sets up a UDP server and listens for connections
func UdpServer(address string, hub *Hub) {
    serverConn := SetupConnection(address)
    defer serverConn.Close()

    hub.sendChannel = make(chan SendPacket, 5)
    addrMap := make(map[string]*Client)
    go Sender(serverConn, hub.sendChannel)
    buf := make([]byte, ConnectionBufferSize)

    rateLoggers := make(map[string]*loggers.RateLogger)

    for {
        n, addr, err := serverConn.ReadFromUDP(buf)
        if err != nil {
            log.WithFields(log.Fields{
                "error": err,
            }).Fatal("UDP read error")
        }
        packet := buf[:n]
        if strings.HasPrefix(string(packet), "CONNECT") {
            NewClient(addr, &packet, &addrMap, hub)
        } else {
            ProcessDataPacket(addr, &packet, &addrMap, &rateLoggers)
        }
    }
}

// SetupConnection sets up the UDP server
func SetupConnection(address string) *net.UDPConn {
    serverAddr, err := net.ResolveUDPAddr("udp", address)
    if err != nil {
        panic(err)
    }
    serverConn, err := net.ListenUDP("udp", serverAddr)
    if err != nil {
        panic(err)
    }
    serverConn.SetReadBuffer(ConnectionBufferSize)
    serverConn.SetWriteBuffer(ConnectionBufferSize)
    return serverConn
}

// NewClient handles connect packets and creates client objects
func NewClient(addr *net.UDPAddr, connectPacket *[]byte, addrMap *map[string]*Client, hub *Hub) {
    split := strings.Split(string(*connectPacket), ":")
    privateKey := split[1]
    name := split[2]
    log.WithFields(log.Fields{
        "privateKey": privateKey,
        "name": name,
    }).Info("New client")
    hub.sendChannel <- SendPacket{addr: addr, data: []byte("HANDSHAKE")}
    if _, ok := hub.clientMap[privateKey]; !ok {
        hub.clientMap[privateKey] = make(map[string]*Client)
    }
    addrStr := string(addr.IP) + ":" + string(addr.Port)
    if _, ok := hub.clientMap[privateKey][name]; !ok {
        cli := &Client{addr: addr, process: make(chan []byte, 1),
        name: name, privateKey: privateKey, hub: hub,
        rateLoggers: make(map[string]*loggers.RateLogger)}
        hub.clientMap[privateKey][name] = cli
        (*addrMap)[addrStr] = cli
        go cli.Processor()
    } else {
        hub.clientMap[privateKey][name].addr = addr
        (*addrMap)[addrStr] = hub.clientMap[privateKey][name]
    }
}

// ProcessDataPacket receives data packets and forwards them to clients
func ProcessDataPacket(addr *net.UDPAddr, packet *[]byte, addrMap *map[string]*Client, rateLoggers *map[string]*loggers.RateLogger) {
    addrStr := string(addr.IP) + ":" + string(addr.Port)
    if _, ok := (*addrMap)[addrStr]; ok {
        if _, ok := (*rateLoggers)[addrStr]; !ok {
            (*rateLoggers)[addrStr] = loggers.NewRateLogger(RateLoggerRate)
        }

        (*rateLoggers)[addrStr].Log("Before process", log.Fields{
            "addr": addr.String(),
        })

        select {
        case (*addrMap)[addrStr].process <- append([]byte{}, *packet...):
        default:
        }
    }
}

// Sender is the thread for sending to clients
func Sender(c *net.UDPConn, s <-chan SendPacket) {
    rateLogger := loggers.NewRateLogger(100)
    for message := range s {
        _, err := c.WriteToUDP(message.data, message.addr)

        rateLogger.Log("Sender frequency", log.Fields{})

        if err != nil {
            log.WithFields(log.Fields{
                "error": err,
            }).Fatal("UDP write error")
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

    h := NewHub()

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
    UdpServer(*addr, h)
}
