package main

import (
    "github.com/garyburd/redigo/redis"
)

type command struct {
    comm string
    args []interface{}
}

type dbwriter struct {
    redisconn *redis.Conn
    commChannel chan command
}

func (d *dbwriter) write(blocking bool, comm string, args ...interface{}) {
    if blocking {
        d.commChannel <- command{comm: comm, args: args}
    } else {
        select {
            case d.commChannel <- command{comm: comm, args: args}:
            default:
        }
    }
}

func (d *dbwriter) writer() {
    for c := range d.commChannel {
        (*d.redisconn).Do(c.comm, c.args...)
    }
}
