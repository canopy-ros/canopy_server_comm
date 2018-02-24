// Package main runs the main server comm program.
package main

import (
	"flag"
	"net"
	"strings"
	"syscall"

	"github.com/canopy-ros/canopy_server_comm/loggers"
	"github.com/garyburd/redigo/redis"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type SendPacket struct {
    addr *net.UDPAddr
    data []byte
}

// hub is a connection point between a network of senders and receivers.
// Communication between senders and receivers is logged to a database
// if a database writer is specified.
type hub struct {
    clientMap   map[string]map[string]*Client
    sendChannel chan SendPacket
	dbw         DBWriter
}

// newHub creates a new hub object.
func newHub() *hub {
	return &hub{
		clientMap: make(map[string]map[string]*Client),
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
	file, _ := serverConn.File()
	defer file.Close()
	syscall.SetsockoptInt(int(file.Fd()), syscall.SOL_IP, 10, 0)
	defer serverConn.Close()

	h.sendChannel = make(chan SendPacket, 5)
	addrMap := make(map[string]*Client)
	go sender(serverConn, h.sendChannel)
	buf := make([]byte, 65507)

	rateLoggers := make(map[string]*loggers.RateLogger)

	for {
		n, addr, err := serverConn.ReadFromUDP(buf)
		if err != nil {
            log.WithFields(log.Fields{
                "error": err,
            }).Fatal("UDP read error")
		}
		if strings.HasPrefix(string(buf[:n]), "CONNECT") {
			split := strings.Split(string(buf[:n]), ":")
			privateKey := split[1]
			name := split[2]
            log.WithFields(log.Fields{
                "privateKey": privateKey,
                "name": name,
            }).Info("New client")
			h.sendChannel <- SendPacket{addr: addr, data: []byte("HANDSHAKE")}
			if _, ok := h.clientMap[privateKey]; !ok {
				h.clientMap[privateKey] = make(map[string]*Client)
			}
			if _, ok := h.clientMap[privateKey][name]; !ok {
				cli := &Client{addr: addr, process: make(chan []byte, 1),
					name: name, privateKey: privateKey, h: h,
					rateLoggers: make(map[string]*loggers.RateLogger)}
				h.clientMap[privateKey][name] = cli
				addrMap[string(addr.IP)+":"+string(addr.Port)] = cli
				go cli.processor()
			} else {
				h.clientMap[privateKey][name].addr = addr
				addrMap[string(addr.IP)+":"+string(addr.Port)] = h.clientMap[privateKey][name]
			}
		} else {
			addrStr := string(addr.IP) + ":" + string(addr.Port)
			if _, ok := addrMap[addrStr]; ok {
				if _, ok := rateLoggers[addrStr]; !ok {
					rateLoggers[addrStr] = loggers.NewRateLogger(100)
				}

				rateLoggers[addrStr].Log("Before process", log.Fields{
					"addr": addr.String(),
				})

				select {
				case addrMap[addrStr].process <- append([]byte{}, buf[:n]...):
				default:
				}
			}
		}
	}
}

func sender(c *net.UDPConn, s <-chan SendPacket) {
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
