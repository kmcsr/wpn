
package wpn

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"github.com/kmcsr/go-logger"
)

var DefaultTransport http.RoundTripper = &http.Transport{
	Proxy: nil, // No proxy because ourself is a proxy
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 20 * time.Second,
	}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          30,
	IdleConnTimeout:       30 * time.Second,
	TLSHandshakeTimeout:   6 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

type Client struct {
	serverURL string
	Logger logger.Logger

	// wpn.Client.Transport will override wpn.Client.WebsocketOpts.HTTPClient.Transport if that is nil
	// Default value is wpn.DefaultTransport
	Transport     http.RoundTripper
	WebsocketOpts *websocket.DialOptions
	IdleTimeout   time.Duration
	MaxIdleConn    int

	oldDialOpts *websocket.DialOptions
	cachedOpts  *websocket.DialOptions

	idleRefreshCh chan struct{}

	ctx    context.Context
	cancel context.CancelFunc

	connMux sync.Mutex
	conns   []*Conn
}

func NewClient(server string)(c *Client){
	c = &Client{
		serverURL: server,
		IdleTimeout: 90 * time.Second,
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

func (c *Client)debugf(format string, args ...any){
	if c.Logger != nil {
		c.Logger.Debugf(format, args...)
	}
}

func (c *Client)Shutdown(){
	c.cancel()
	c.connMux.Lock()
	defer c.connMux.Unlock()

	for _, conn := range c.conns {
		conn.Close()
	}
	c.conns = nil
}

func (c *Client)Ping()(t time.Duration, err error){
	conn, err := c.getIdleConn()
	if err != nil {
		return
	}
	defer conn.Unlock()
	return conn.Ping()
}

func (c *Client)checkConns(){
	c.connMux.Lock()
	defer c.connMux.Unlock()

	if c.ctx.Err() != nil {
		return
	}

	idleRemain := 0
	i, j := 0, len(c.conns)
	for i < j {
		for i < j {
			conn := c.conns[i]
			if conn.Closed() {
				break
			}
			if conn.Status() == StatusIdle {
				if idleRemain >= c.MaxIdleConn {
					conn.Close()
					break
				}
				idleRemain++
			}
			i++
		}
		for i < j { // idleRemain always greater than zero
			j--
			conn := c.conns[j]
			if conn.Status() == StatusIdle {
				conn.Close()
			}else if !conn.Closed() {
				c.conns[i] = conn
				break
			}
		}
	}
	c.conns = c.conns[:j]
	if idleRemain == 0 {
		c.addIdleConn()
	}
}

func (c *Client)getWebsocketOpts()(opts *websocket.DialOptions){
	if c.cachedOpts != nil && c.oldDialOpts == c.WebsocketOpts {
		return c.cachedOpts
	}
	if c.WebsocketOpts == nil {
		opts = new(websocket.DialOptions)
	}else{
		opts = &*c.WebsocketOpts
	}
	transport := c.Transport
	if transport == nil {
		transport = DefaultTransport
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{
			Transport: transport,
		}
	}else if opts.HTTPClient.Transport == nil {
		opts.HTTPClient = &*opts.HTTPClient
		opts.HTTPClient.Transport = DefaultTransport
	}
	c.cachedOpts = opts
	return
}

func (c *Client)newConn()(conn *Conn, err error){
	var ws *websocket.Conn
	if ws, _, err = websocket.Dial(c.ctx, c.serverURL, c.getWebsocketOpts()); err != nil {
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
	select {
	case c.idleRefreshCh <- struct{}{}:
	case <-c.ctx.Done():
		return nil, context.Cause(c.ctx)
	}

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
	c.debugf("Dialing %s: %q", protocol, target)
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
