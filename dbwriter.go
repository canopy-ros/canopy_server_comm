package main

import (
	"github.com/garyburd/redigo/redis"
)

// db writer interface
type dbwriter interface {
	writer()
	addKey(blocking bool, key string, value interface{})
	deleteKey(blocking bool, key string)
	setAdd(blocking bool, key string, members ...interface{})
	setRemove(blocking bool, key string, members ...interface{})
	closeConn()
}

// redis db writer
type command struct {
	comm string
	args []interface{}
}

type redisWriter struct {
	conn        *redis.Conn
	commChannel chan command
}

func (rw *redisWriter) write(blocking bool, comm string, args ...interface{}) {
	if blocking {
		rw.commChannel <- command{comm: comm, args: args}
	} else {
		select {
		case rw.commChannel <- command{comm: comm, args: args}:
		default:
		}
	}
}

func (rw *redisWriter) writer() {
	for c := range rw.commChannel {
		(*rw.conn).Do(c.comm, c.args...)
	}
}

func (rw *redisWriter) addKey(blocking bool, key string, value interface{}) {
	rw.write(blocking, "SET", key, value)
}

func (rw *redisWriter) deleteKey(blocking bool, key string) {
	rw.write(blocking, "DEL", key)
}

func (rw *redisWriter) setAdd(blocking bool, key string, members ...interface{}) {
	rw.write(blocking, "SADD", key, members)
}

func (rw *redisWriter) setRemove(blocking bool, key string, members ...interface{}) {
	rw.write(blocking, "SREM", members)
}

func (rw *redisWriter) closeConn() {
	(*rw.conn).Close()
}
