
package wpn

import (
	"context"
	"net"
	"net/http"
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

	ctx    context.Context
	cancel context.CancelFunc

	idleConns   connPool
	streamConns connPool
	packetConns connPool
}

func NewClient(server string)(c *Client){
	c = &Client{
		serverURL: server,
		IdleTimeout: 90 * time.Second,
		MaxIdleConn: 5,
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	for i := 0; i < c.MaxIdleConn; i++ {
		c.addIdleConn()
	}
	return
}

func (c *Client)debugf(format string, args ...any){
	if c.Logger != nil {
		c.Logger.Debugf(format, args...)
	}
}

func (c *Client)checkConns()(ok bool){
	for _, conn := range c.idleConns.Clear() {
		conn.Close()
	}
	return c.streamConns.Len() == 0 && c.packetConns.Len() == 0
}

func (c *Client)isShutingdown()(bool){
	return c.ctx.Err() != nil
}

func (c *Client)Shutdown(ctx context.Context)(err error){
	c.cancel()

	const pollInterval = time.Millisecond * 50
	for !c.checkConns() {
		select {
		case <-ctx.Done():
			for _, conn := range c.idleConns.Clear() {
				conn.Close()
			}
			for _, conn := range c.streamConns.Clear() {
				conn.Close()
			}
			for _, conn := range c.packetConns.Clear() {
				conn.Close()
			}
			return context.Cause(ctx)
		case <-time.After(pollInterval):
		}
	}
	return
}

func (c *Client)Ping()(t time.Duration, err error){
	conn, err := c.getIdleConn()
	if err != nil {
		return
	}
	return conn.Ping()
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
	c.idleConns.Put(conn)
	return
}

func (c *Client)getIdleConn()(conn *Conn, err error){
	conn = c.idleConns.TakeOr(func()(conn *Conn){
		conn, err = c.newConn()
		return
	})
	if c.idleConns.Len() * 2 < c.MaxIdleConn {
		go c.addIdleConn()
	}
	return
}

func (c *Client)DialContext(ctx context.Context, protocol string, target string)(pipe net.Conn, err error){
	conn, err := c.getIdleConn()
	if err != nil {
		return
	}
	defer func(){
		if c.isShutingdown() {
			conn.Close()
			if err == nil {
				err = context.Cause(c.ctx)
			}
		}
	}()
	c.debugf("Dialing %s: %q", protocol, target)
	var pp *ConnPipe
	if pp, err = conn.DialContext(ctx, protocol, target); err != nil {
		c.idleConns.Put(conn)
		return
	}
	c.streamConns.Put(conn)
	go func(){
		select {
		case <-c.ctx.Done():
			return
		case <-pp.AfterClose():
			c.idleConns.PutFrom(conn, &c.streamConns)
		}
	}()
	pipe = pp
	return
}

func (c *Client)Dial(protocol string, target string)(pipe net.Conn, err error){
	return c.DialContext(context.Background(), protocol, target)
}

func (c *Client)ListenPacketContext(ctx context.Context, protocol string, target string)(pipe net.PacketConn, err error){
	conn, err := c.getIdleConn()
	if err != nil {
		return
	}
	defer func(){
		if e := c.ctx.Err(); e != nil {
			conn.Close()
			if err == nil {
				err = e
			}
		}
	}()
	c.debugf("Listening %s: %q", protocol, target)
	var pp *ConnPacketPipe
	if pp, err = conn.ListenPacketContext(ctx, protocol, target); err != nil {
		c.idleConns.Put(conn)
		return
	}
	c.packetConns.Put(conn)
	go func(){
		select {
		case <-c.ctx.Done():
			return
		case <-pp.AfterClose():
			c.idleConns.PutFrom(conn, &c.packetConns)
		}
	}()
	pipe = pp
	return
}

func (c *Client)ListenPacket(protocol string, target string)(pipe net.PacketConn, err error){
	return c.ListenPacketContext(context.Background(), protocol, target)
}
