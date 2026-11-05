package grpcx

import (
	"errors"
	"net"
	"sync"
)

// LimitListener bounds active connections. Closing it unblocks Accept; accepted connections release capacity on Close.
func LimitListener(l net.Listener, n int) (net.Listener, error) {
	if l == nil || n < 1 {
		return nil, errors.New("grpcx: listener and positive connection limit required")
	}
	return &limitedListener{Listener: l, slots: make(chan struct{}, n), done: make(chan struct{})}, nil
}

type limitedListener struct {
	net.Listener
	slots chan struct{}
	done  chan struct{}
	once  sync.Once
}

func (l *limitedListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
	case <-l.done:
		return nil, net.ErrClosed
	}
	conn, e := l.Listener.Accept()
	if e != nil {
		<-l.slots
		return nil, e
	}
	return &limitedConn{Conn: conn, release: func() { <-l.slots }}, nil
}
func (l *limitedListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}

type limitedConn struct {
	net.Conn
	once    sync.Once
	release func()
}

func (c *limitedConn) Close() error { e := c.Conn.Close(); c.once.Do(c.release); return e }
