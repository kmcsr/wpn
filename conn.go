
package wpn

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"github.com/kmcsr/wpn/internal/pool"
)

var (
	ErrIllegalStatus = errors.New("Illegal status error")
)

type RemoteDialError struct {
	Message string
}

func (e *RemoteDialError)Error()(string){
	return fmt.Sprintf("wpn: remote dial error: %s", e.Message)
}

type StatusError struct {
	Status ConnStatus
}

func (e *StatusError)Error()(string){
	return fmt.Sprintf("wpn: Conn.status must be StatusIdle, but got %d", e.Status)
}


type ConnStatus byte

const (
	// The connection is idle
	StatusIdle   ConnStatus = 0x00
	// The connection is used for packet based internet, such as IP, UDP
	StatusPacket ConnStatus = 0x01
	// The connection is used for stream based internet, such as TCP
	StatusStream ConnStatus = 0x02

	// The connection is in an operation
	StatusInOper ConnStatus = 0xfe
	// The connection is closed
	StatusClosed ConnStatus = 0xff
)

type (
	Dialer func(ctx context.Context, protocol string, target string)(net.Conn, error)
	PacketListener func(ctx context.Context, protocol string, target string)(net.PacketConn, error)
)

func dialContext(ctx context.Context, protocol string, target string)(net.Conn, error){
	return net.Dial(protocol, target)
}

func listenPacketContext(ctx context.Context, protocol string, target string)(net.PacketConn, error){
	return net.ListenPacket(protocol, target)
}

var (
	_ Dialer = dialContext
	_ PacketListener = listenPacketContext
)

type Conn struct {
	PingInterval time.Duration

	Dialer         Dialer
	PacketListener PacketListener

	ctx    context.Context
	cancel context.CancelCauseFunc
	err    error

	ws     *websocket.Conn
	status ConnStatus
	lock   sync.Mutex

	onResult func(ok bool, msg string)
	onCmdErr func(msg string)
	// If the function is not `nil`, and it returned false, the incoming connection will be blocked
	OnCheckIncoming func(protocol string, target string)(bool)

	// stream connection, only when `status == StatusStream`
	streamConn net.Conn
	// packet connection, only when `status == StatusPacket` and on server side
	packetConn net.PacketConn
	// packetConn received data, only on client side
	packetDataCh chan *packetDataT
}

type packetDataT struct {
	buf []byte
	freeBuf func()
	addr net.Addr
	err error
}

// Wrap a websocket connection to wpn connection
// You should call `Conn.Handle` later to process datas
func WrapConn(ctx context.Context, ws *websocket.Conn)(c *Conn){
	c = &Conn{
		PingInterval: time.Second * 3,
		ws: ws,
		status: StatusIdle,
	}
	c.ctx, c.cancel = context.WithCancelCause(ctx)
	return
}

// return the latest error cause the connection closed
//  or the close handshake error (maybe nil) if there are no error during the connection is open
func (c *Conn)Close()(err error){
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.status == StatusClosed {
		return c.err
	}
	c.status = StatusClosed
	if c.streamConn != nil {
		c.streamConn.Close()
		c.streamConn = nil
	}
	if c.packetConn != nil {
		c.packetConn.Close()
		c.packetConn = nil
	}
	err = c.ws.Close(websocket.StatusNormalClosure, "")
	c.cancel(nil)
	return
}

func (c *Conn)Closed()(bool){
	return c.status == StatusClosed
}

func (c *Conn)Status()(ConnStatus){
	return c.status
}

func (c *Conn)closeWithErr(err error){
	c.closeWithCode(websocket.StatusInternalError, err)
}

func (c *Conn)closeWithCode(code websocket.StatusCode, err error){
	if c.status == StatusClosed {
		return
	}
	c.status = StatusClosed
	if c.streamConn != nil {
		c.streamConn.Close()
		c.streamConn = nil
	}
	if c.packetConn != nil {
		c.packetConn.Close()
		c.packetConn = nil
	}
	var msg string
	if err == nil {
		if code != websocket.StatusNormalClosure {
			err = fmt.Errorf("Closed with status = %d", code)
			c.err = err
		}
	}else{
		c.err = err
		msg = err.Error()
	}
	c.ws.Close(code, msg)
	c.cancel(err)
}

const (
	cmdClose      = 'X'
	cmdOpenPacket = 'P'
	cmdOpenStream = 'S'
	cmdReply      = 'R'
	cmdError      = 'E'

	cmdTrue  = 'T'
	cmdFalse = 'F'

	cmdCloseS      = "X"
	cmdOpenPacketS = "P"
	cmdOpenStreamS = "S"
	cmdReplyS      = "R"
	cmdErrorS      = "E"

	cmdRTrueS  = cmdReplyS + "T"
	cmdRFalseS = cmdReplyS + "F"
)

// will automatically close connection if an error occurred during write
func (c *Conn)sendCmd(cmd string)(err error){
	if err = c.ws.Write(c.ctx, websocket.MessageText, ([]byte)(cmd)); err != nil {
		c.closeWithErr(err)
		return
	}
	return
}

func (c *Conn)handleCommand(cmd string)(err error){
	c.lock.Lock()
	defer c.lock.Unlock()

	switch cmd[0] {
	case cmdClose:
		c.status = StatusIdle
		if c.streamConn != nil {
			c.streamConn.Close()
			c.streamConn = nil
		}
		if err = c.sendCmd(cmdReplyS); err != nil {
			c.closeWithErr(err)
			return
		}
	case cmdOpenPacket:
		if c.status != StatusIdle {
			c.closeWithCode(websocket.StatusInvalidFramePayloadData, ErrIllegalStatus)
			return
		}
		protocol, target := split(cmd[1:], ';')
		if c.OnCheckIncoming != nil && !c.OnCheckIncoming(protocol, target) {
			if err = c.sendCmd(cmdRFalseS + "Incoming request blocked"); err != nil {
				return
			}
			return
		}
		listener := c.PacketListener
		if listener == nil {
			listener = listenPacketContext
		}
		var conn net.PacketConn
		if conn, err = listener(c.ctx, protocol, target); err != nil {
			if err = c.sendCmd(cmdRFalseS + err.Error()); err != nil {
				return
			}
			return
		}
		c.status = StatusPacket
		c.packetConn = conn
		if err = c.sendCmd(cmdRTrueS + conn.LocalAddr().String()); err != nil {
			return
		}
		go c.servePacket()
	case cmdOpenStream:
		if c.status != StatusIdle {
			c.closeWithCode(websocket.StatusInvalidFramePayloadData, ErrIllegalStatus)
			return
		}
		protocol, target := split(cmd[1:], ';')
		if c.OnCheckIncoming != nil && !c.OnCheckIncoming(protocol, target) {
			if err = c.sendCmd(cmdRFalseS + "Incoming request blocked"); err != nil {
				return
			}
			return
		}
		dialer := c.Dialer
		if dialer == nil {
			dialer = dialContext
		}
		var conn net.Conn
		if conn, err = dialer(c.ctx, protocol, target); err != nil {
			if err = c.sendCmd(cmdRFalseS + err.Error()); err != nil {
				return
			}
			return
		}
		c.status = StatusStream
		c.streamConn = conn
		if err = c.sendCmd(cmdRTrueS + conn.LocalAddr().String()); err != nil {
			return
		}
		go c.serveStream()
	case cmdReply:
		if c.onResult != nil {
			if len(cmd) == 1 {
				c.onResult(false, "")
			}else{
				c.onResult(cmd[1] == cmdTrue, cmd[2:])
			}
		}
	case cmdError:
		if c.status != StatusIdle {
			if c.onCmdErr != nil {
				c.onCmdErr(cmd[1:])
			}
		}
	}
	return
}

func (c *Conn)handleBinary(r io.Reader)(err error){
	c.lock.Lock()
	defer c.lock.Unlock()

	switch c.status {
	case StatusPacket:
		var addr stringAddr
		br := bufio.NewReader(r)
		var l byte
		if l, err = br.ReadByte(); err != nil {
			return
		}
		buf := make([]byte, l)
		if _, err = io.ReadFull(br, buf); err != nil {
			return
		}
		addr.network = (string)(buf)
		if l, err = br.ReadByte(); err != nil {
			return
		}
		buf = make([]byte, l)
		if _, err = io.ReadFull(br, buf); err != nil {
			return
		}
		addr.addr = (string)(buf)
		data := &packetDataT{
			addr: &addr,
		}
		buf, data.freeBuf = pool.GetIPPacketBuf()
		var n int
		if n, err = br.Read(buf); err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}else{
				data.err = err
			}
		}
		data.buf = buf[:n]
		go func(){
			select {
			case <-c.ctx.Done():
				return
			case c.packetDataCh <- data:
			}
		}()
	case StatusStream:
		if c.streamConn == nil {
			return
		}
		_, err = io.Copy(c.streamConn, r)
		if err != nil {
			c.status = StatusIdle
			c.streamConn.Close()
			c.streamConn = nil
			if err = c.sendCmd(cmdErrorS + err.Error()); err != nil {
				return
			}
		}
	}
	return
}

// will automatically close connection if an error occurred
func (c *Conn)Handle()(err error){
	if c.PingInterval > 0 {
		go func(){
			for c.PingInterval > 0 {
				select {
				case <-c.ctx.Done():
					c.status = StatusClosed
					c.ws.Close(websocket.StatusInternalError, context.Cause(c.ctx).Error())
					return
				case <-time.After(c.PingInterval):
					if err := c.ws.Ping(c.ctx); err != nil {
						c.closeWithErr(err)
						return
					}
				}
			}
		}()
	}
	for {
		var (
			typ websocket.MessageType
			r io.Reader
		)
		typ, r, err = c.ws.Reader(c.ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
				c.cancel(nil)
			}else if ce := new(websocket.CloseError); errors.As(err, ce) && ce.Code == websocket.StatusNormalClosure {
				err = nil
				c.cancel(nil)
			}else{
				c.closeWithErr(err)
			}
			return
		}
		if typ == websocket.MessageText {
			var buf []byte
			buf, err = io.ReadAll(r)
			if err != nil {
				c.closeWithErr(err)
				return
			}
			if err = c.handleCommand((string)(buf)); err != nil {
				return
			}
		}else{ // if typ == websocket.MessageBinary
			if err = c.handleBinary(r); err != nil {
				return
			}
		}
	}
}

// `Ping` will ping the remote point, return the time used and possible error
// If an error occured during ping, the connection will be automaticly closed
// The connection can ping with any status
func (c *Conn)Ping()(t time.Duration, err error){
	before := time.Now()
	if err = c.ws.Ping(c.ctx); err != nil {
		c.closeWithErr(err)
		return
	}
	t = time.Since(before)
	return
}

func (c *Conn)closeAndWaitForIdle()(err error){
	done := make(chan struct{}, 0)

	c.lock.Lock()
	c.onResult = func(ok bool, msg string){
		c.onResult = nil
		c.status = StatusIdle
		close(done)
	}
	c.lock.Unlock()
	if err = c.sendCmd(cmdCloseS); err != nil {
		return
	}
	select {
	case <-c.ctx.Done():
		return context.Cause(c.ctx)
	case <-done:
	}
	return
}

// `DialContext` will only success if `status` is `StatusIdle`
// `Conn.Handle` must be called concurrently with `DialContext`
func (c *Conn)DialContext(ctx context.Context, protocol string, target string)(pipe *ConnPipe, err error){
	c.lock.Lock()

	if c.status != StatusIdle {
		err = &StatusError{c.status}
		c.lock.Unlock()
		return
	}

	type resultT struct {
		ok bool
		msg string
	}
	resCh := make(chan resultT, 1)
	c.onResult = func(ok bool, msg string){
		c.onResult = nil
		resCh <- resultT{ok, msg}
	}
	if err = c.sendCmd(cmdOpenStreamS + protocol + ";" + target); err != nil {
		c.lock.Unlock()
		return
	}
	c.status = StatusInOper

	c.lock.Unlock()

	var addr stringAddr

	select {
	case res := <-resCh:
		if res.ok {
			addr.network, addr.addr = split(res.msg, ';')
		}else{
			err = &RemoteDialError{res.msg}
			c.status = StatusIdle
			return
		}
	case <-ctx.Done():
		c.lock.Lock()
		defer c.lock.Unlock()

		c.status = StatusIdle
		if c.onResult == nil { // just then, the connection accepted, so we have to close it
			if err = c.closeAndWaitForIdle(); err != nil {
				return
			}
		}else{
			c.onResult = nil
		}
		return nil, context.Cause(ctx)
	case <-c.ctx.Done():
		return nil, context.Cause(c.ctx)
	}

	c.lock.Lock()

	c.streamConn, pipe = newConnPipe()
	pipe.localAddr = &addr
	c.onCmdErr = func(msg string){
		c.onCmdErr = nil
		pipe.Close()
		// TODO: return error msg
	}
	c.status = StatusStream
	go c.serveStream()

	c.lock.Unlock()
	return
}

// `DialContext` will only success if `status` is `StatusIdle` or `StatusLocked`
// `Conn.Handle` must be called concurrently with `DialContext`
func (c *Conn)Dial(protocol string, target string)(pipe *ConnPipe, err error){
	return c.DialContext(context.Background(), protocol, target)
}

func (c *Conn)serveStream()(err error){
	conn := c.streamConn
	if c.status != StatusStream {
		return
	}
	defer conn.Close()
	buf, freeBuf := pool.GetIPPacketBuf()
	defer freeBuf()
	for {
		var n int
		if n, err = conn.Read(buf); err != nil {
			c.lock.Lock()
			ok := c.status == StatusStream
			c.lock.Unlock()
			if !ok {
				return nil
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				return c.closeAndWaitForIdle()
			}
			return c.sendCmd(cmdErrorS + err.Error())
		}
		if err = c.ws.Write(c.ctx, websocket.MessageBinary, buf[:n]); err != nil {
			c.closeWithErr(err)
			return
		}
	}
}

func (c *Conn)sendPacketData(ctx context.Context, data []byte, addr net.Addr)(err error){
	if ctx == nil {
		ctx = c.ctx
	}
	buf := packetFormatBufPool.Get().(*bytes.Buffer)
	defer packetFormatBufPool.Put(buf)
	buf.Reset()
	buf.WriteByte((byte)(len(addr.Network())))
	buf.WriteString(addr.Network())
	buf.WriteByte((byte)(len(addr.String())))
	buf.WriteString(addr.String())
	buf.Write(data)
	if err = c.ws.Write(ctx, websocket.MessageBinary, buf.Bytes()); err != nil {
		c.closeWithErr(err)
		return
	}
	return
}

// should only call on server side
func (c *Conn)servePacket()(err error){
	conn := c.packetConn
	if c.status != StatusPacket {
		return
	}
	defer conn.Close()
	buf, freeBuf := pool.GetIPPacketBuf()
	defer freeBuf()
	for {
		var (
			n int
			addr net.Addr
		)
		if n, addr, err = conn.ReadFrom(buf); err != nil {
			c.lock.Lock()
			ok := c.status == StatusPacket
			c.lock.Unlock()
			if !ok {
				return nil
			}
			return c.sendCmd(cmdErrorS + err.Error())
		}
		if err = c.sendPacketData(nil, buf[:n], addr); err != nil {
			return
		}
	}
}

// ConnPacketPipe only used on the client side
type ConnPacketPipe struct {
	conn *Conn
	localAddr net.Addr
	closer func()
	closed chan struct{}
	readDeadline, writeDeadline time.Time
}

var _ net.PacketConn = (*ConnPacketPipe)(nil)

func newConnPacketPipe(c *Conn)(p *ConnPacketPipe){
	p = &ConnPacketPipe{
		conn: c,
	}
	ch := make(chan struct{}, 0)
	p.closed = ch
	p.closer = sync.OnceFunc(func(){
		close(ch)
	})
	return
}

func (p *ConnPacketPipe)Close()(error){
	p.conn = nil
	p.closer()
	return nil
}

func (p *ConnPacketPipe)ReadFrom(buf []byte)(n int, addr net.Addr, err error){
	ctx := p.conn.ctx
	if !p.readDeadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, p.readDeadline)
		defer cancel()
	}

	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case res := <-p.conn.packetDataCh:
		if res == nil {
			return 0, nil, context.Canceled
		}
		n = copy(buf, res.buf)
		res.freeBuf()
		addr, err = res.addr, res.err
		return
	}
}

// The results of Network() and String() of addr must less than 256 bytes
func (p *ConnPacketPipe)WriteTo(buf []byte, addr net.Addr)(n int, err error){
	ctx := p.conn.ctx
	if !p.writeDeadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, p.writeDeadline)
		defer cancel()
	}

	if err = p.conn.sendPacketData(ctx, buf, addr); err != nil {
		return
	}
	return
}

func (p *ConnPacketPipe)AfterClose()(<-chan struct{}){
	return p.closed
}

func (p *ConnPacketPipe)LocalAddr()(net.Addr){
	return p.localAddr
}

func (p *ConnPacketPipe)SetDeadline(t time.Time)(error){
	p.readDeadline = t
	p.writeDeadline = t
	return nil
}

func (p *ConnPacketPipe)SetReadDeadline(t time.Time)(error){
	p.readDeadline = t
	return nil
}

func (p *ConnPacketPipe)SetWriteDeadline(t time.Time)(error){
	p.writeDeadline = t
	return nil
}

func (c *Conn)ListenPacketContext(ctx context.Context, protocol string, target string)(pipe *ConnPacketPipe, err error){
	c.lock.Lock()

	if c.status != StatusIdle {
		err = &StatusError{c.status}
		c.lock.Unlock()
		return
	}

	type resultT struct {
		ok bool
		msg string
	}
	resCh := make(chan resultT, 1)
	c.onResult = func(ok bool, msg string){
		c.onResult = nil
		resCh <- resultT{ok, msg}
	}
	if err = c.sendCmd(cmdOpenPacketS + protocol + ";" + target); err != nil {
		c.lock.Unlock()
		return
	}
	c.status = StatusInOper
	c.packetDataCh = make(chan *packetDataT, 3)
	c.lock.Unlock()

	var addr stringAddr

	select {
	case res := <-resCh:
		if res.ok {
			addr.network, addr.addr = split(res.msg, ';')
		}else{
			err = &RemoteDialError{res.msg}
			c.status = StatusIdle
			return
		}
	case <-ctx.Done():
		c.lock.Lock()
		defer c.lock.Unlock()

		c.status = StatusIdle
		if c.onResult == nil { // just then, the connection accepted, so we have to close it
			if err = c.closeAndWaitForIdle(); err != nil {
				return
			}
		}else{
			c.onResult = nil
		}
		close(c.packetDataCh)
		c.packetDataCh = nil
		return nil, context.Cause(ctx)
	case <-c.ctx.Done():
		return nil, context.Cause(c.ctx)
	}

	c.lock.Lock()

	pipe = newConnPacketPipe(c)
	pipe.localAddr = &addr
	c.onCmdErr = func(msg string){
		c.onCmdErr = nil
		pipe.Close()
		close(c.packetDataCh)
		c.packetDataCh = nil
		// TODO: return error msg
	}
	c.status = StatusPacket

	c.lock.Unlock()
	return 
}

func (c *Conn)ListenPacket(protocol string, target string)(pipe *ConnPacketPipe, err error){
	return c.ListenPacketContext(context.Background(), protocol, target)
}
