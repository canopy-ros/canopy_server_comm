package main

import (
	"github.com/garyburd/redigo/redis"
)

// dbwriter is an interface for database writers.
type dbwriter interface {
	writer()
	addKey(blocking bool, key string, value interface{})
	deleteKey(blocking bool, key ...string)
	setAdd(blocking bool, key string, members ...interface{})
	setRemove(blocking bool, key string, members ...interface{})
	closeConn()
}

// command consists of a command and arguments to be sent to
// a database with a dbwriter.
type command struct {
	comm string
	args []interface{}
}

// redisWriter is a dbwriter for redis.
type redisWriter struct {
	conn        *redis.Conn
	commChannel chan command
}

// write from redisWriter sends specified commands and arguments
// to the object's communication channel.
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

// writer from redisWriter sends commands and arguments from the
// object's communication channel to the redis database.
func (rw *redisWriter) writer() {
	for c := range rw.commChannel {
		(*rw.conn).Do(c.comm, c.args...)
	}
}

// addKey from redisWriter adds a specified key and value to redis.
func (rw *redisWriter) addKey(blocking bool, key string, value interface{}) {
	rw.write(blocking, "SET", key, value)
}

// deleteKey from redisWriter deletes a specified key from redis.
func (rw *redisWriter) deleteKey(blocking bool, key ...string) {
	rw.write(blocking, "DEL", key)
}

// setAdd from redisWriter adds members to a set in redis.
func (rw *redisWriter) setAdd(blocking bool, key string, members ...interface{}) {
	rw.write(blocking, "SADD", key, members)
}

// setRemove from redisWriter removes members from a set in redis.
func (rw *redisWriter) setRemove(blocking bool, key string, members ...interface{}) {
	rw.write(blocking, "SREM", members)
}

// closeConn from redisWriter closes the connection to redis.
func (rw *redisWriter) closeConn() {
	(*rw.conn).Close()
}
