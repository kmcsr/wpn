
package wpn

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

type Client struct {
	serverURL string
	WebsocketOpts *websocket.DialOptions

	IdleTimeout time.Duration
	MaxIdleConn int

	idleRefreshCh chan struct{}

	ctx    context.Context
	cancel context.CancelFunc

	connMux sync.Mutex
	conns   []*Conn
}

func NewClient(server string)(c *Client){
	c = &Client{
		serverURL: server,
		IdleTimeout: time.Minute,
		MaxIdleConn: 5,
		idleRefreshCh: make(chan struct{}, 1),
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	go func(){
		for {
			if c.IdleTimeout > 0 {
				select {
				case <-c.ctx.Done():
					return
				case <-time.After(c.IdleTimeout):
					c.checkConns()
				case <-c.idleRefreshCh:
				}
			}else{
				select {
				case <-c.ctx.Done():
					return
				case <-c.idleRefreshCh:
				}
			}
		}
	}()
	for i := 0; i < 3; i++ {
		c.addIdleConn()
	}
	return
}

func (c *Client)checkConns(){
	c.connMux.Lock()
	defer c.connMux.Unlock()

	println("checking conn")
	defer println("checking conn done")

	idleRemain := 0
	i, j := 0, len(c.conns)
	for i < j {
		for i < j {
			conn := c.conns[i]
			if conn.Closed() {
				break
			}
			if conn.Status() == StatusIdle {
				println("idleRemain:", idleRemain, i, j)
				if idleRemain >= c.MaxIdleConn {
					conn.Close()
					break
				}
				idleRemain++
			}
			println("status:", conn.Status(), i, j)
			i++
		}
		println("i:", i)
		for i < j { // idleRemain always greater than zero
			j--
			conn := c.conns[j]
			if conn.Status() == StatusIdle {
				conn.Close()
			}else if !conn.Closed() {
				c.conns[i] = conn
				break
			}else{
				println("idle", i, j)
			}
		}
	}
	c.conns = c.conns[:j]
	if idleRemain == 0 {
		c.addIdleConn()
	}
}

func (c *Client)newConn()(conn *Conn, err error){
	var ws *websocket.Conn
	if ws, _, err = websocket.Dial(c.ctx, c.serverURL, c.WebsocketOpts); err != nil {
		return
	}
	conn = WrapConn(c.ctx, ws)
	// always prevent incoming connection for client
	conn.OnCheckIncoming = func(protocol string, target string)(bool){
		return false
	}
	go conn.Handle()
	return
}

func (c *Client)addIdleConn()(err error){
	conn, err := c.newConn()
	if err != nil {
		return
	}
	c.conns = append(c.conns, conn)
	return
}

func (c *Client)getIdleConn()(conn *Conn, err error){
	c.idleRefreshCh <- struct{}{}
	c.connMux.Lock()
	defer c.connMux.Unlock()
	for i := len(c.conns) - 1; i >= 0; i-- {
		conn = c.conns[i]
		if conn.Lock() {
			return
		}
	}
	if conn, err = c.newConn(); err != nil {
		return
	}
	if !conn.Lock() {
		return nil, errors.New("Assert: Idle connection lock failed")
	}
	c.conns = append(c.conns, conn)
	c.addIdleConn()
	return
}

func (c *Client)DialContext(ctx context.Context, protocol string, target string)(rw net.Conn, err error){
	conn, err := c.getIdleConn()
	if err != nil {
		return
	}
	var pipe *ConnPipe
	if pipe, err = conn.DialContext(ctx, protocol, target); err != nil {
		return
	}
	go func(){
		c.idleRefreshCh <- <- pipe.AfterClose()
	}()
	rw = pipe
	return
}

func (c *Client)Dial(protocol string, target string)(rw net.Conn, err error){
	return c.DialContext(context.Background(), protocol, target)
}
