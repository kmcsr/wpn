
package wpn

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

var bufPool = sync.Pool{
	New: func()(any){
		res := new([]byte)
		*res = make([]byte, 8192)
		return res
	},
}

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

	// The connection is locked for later use
	StatusLocked ConnStatus = 0xfe
	// The connection is closed
	StatusClosed ConnStatus = 0xff
)

type Dialer func(ctx context.Context, protocol string, target string)(net.Conn, error)

func dialContext(ctx context.Context, protocol string, target string)(net.Conn, error){
	return net.Dial(protocol, target)
}

var _ Dialer = dialContext

type Conn struct {
	PingInterval time.Duration
	Dialer Dialer

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

	// stream connection, only when `typ == ConnStream`
	streamConn net.Conn
}

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
	c.status = StatusClosed
	if c.streamConn != nil {
		c.streamConn.Close()
		c.streamConn = nil
	}
	c.lock.Unlock()
	if c.err != nil {
		return c.err
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

// Lock the connection for later use on clientside only
func (c *Conn)Lock()(ok bool){
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.status != StatusIdle {
		return false
	}
	c.status = StatusLocked
	return true
}

// Unlock will unlock the connection if it's locked and return true, or it will return false
func (c *Conn)Unlock()(ok bool){
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.status != StatusLocked {
		return false
	}
	c.status = StatusIdle
	return true
}

func (c *Conn)closeWithErr(err error){
	c.closeWithCode(websocket.StatusInternalError, err)
}

func (c *Conn)closeWithCode(code websocket.StatusCode, err error){
	if c.status != StatusClosed {
		c.status = StatusClosed
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
		c.status = StatusPacket
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
		if err = c.sendCmd(cmdRTrueS + conn.LocalAddr().String()); err != nil {
			conn.Close()
			return
		}
		c.status = StatusStream
		c.streamConn = conn
		go c.serveStream(conn)
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
		// TODO
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

// `DialContext` will only success if `status` is `StatusIdle` or `StatusLocked`
// `Conn.Handle` must be called concurrently with `DialContext`
func (c *Conn)DialContext(ctx context.Context, protocol string, target string)(pipe *ConnPipe, err error){
	c.lock.Lock()

	isidle := c.status == StatusIdle
	if !isidle && c.status != StatusLocked {
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
	c.status = StatusLocked

	c.lock.Unlock()

	var localAddr string

	select {
	case res := <-resCh:
		if res.ok {
			localAddr = res.msg
		}else{
			err = &RemoteDialError{res.msg}
			if isidle {
				c.status = StatusIdle
			}
			return
		}
	case <-ctx.Done():
		if isidle {
			c.status = StatusIdle
		}
		return nil, context.Cause(ctx)
	case <-c.ctx.Done():
		if isidle {
			c.status = StatusIdle
		}
		return nil, context.Cause(c.ctx)
	}

	c.lock.Lock()

	c.streamConn, pipe = newConnPipe()
	pipe.localAddr = &stringTCPAddr{localAddr}
	c.onCmdErr = func(msg string){
		c.onCmdErr = nil
		pipe.Close()
		// TODO: return msg
	}
	c.status = StatusStream
	go c.serveStream(c.streamConn)

	c.lock.Unlock()
	return pipe, nil
}

// `DialContext` will only success if `status` is `StatusIdle` or `StatusLocked`
// `Conn.Handle` must be called concurrently with `DialContext`
func (c *Conn)Dial(protocol string, target string)(pipe *ConnPipe, err error){
	return c.DialContext(context.Background(), protocol, target)
}

// pass the copy of c.streamConn as the argument to avoid strange concurrent problem
func (c *Conn)serveStream(conn net.Conn)(err error){
	defer conn.Close()
	bufP := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufP)
	buf := *bufP
	for {
		var n int
		n, err = conn.Read(buf)
		if err != nil {
			c.lock.Lock()
			ok := c.status == StatusStream
			c.lock.Unlock()
			if ok {
				if errors.Is(err, io.EOF) {
					c.onResult = func(ok bool, msg string){
						c.onResult = nil
						c.status = StatusIdle
					}
					return c.sendCmd(cmdCloseS)
				}
				return c.sendCmd(cmdErrorS + err.Error())
			}
			return nil
		}
		if err = c.ws.Write(c.ctx, websocket.MessageBinary, buf[:n]); err != nil {
			c.closeWithErr(err)
			return
		}
	}
}
