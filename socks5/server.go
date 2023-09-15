
package socks5

import (
	"io"
	"net"
)

const (
	CmdDialTCP    = 0x01
	CmdBindTCP    = 0x02
	CmdForwardUDP = 0x03
)

//
// if Handler returned ErrUnsupportCommand, the server will reply with StatusCommandUnsupported
//
type Handler interface {
	DialTCP(addr *AddrPort)(net.Conn, error)
	BindTCP(addr *AddrPort)(net.Listener, error)
	BindUDP(addr *AddrPort)(net.PacketConn, error)
}

const DefaultSocksPort = ":1080"

type Server struct {
	Addr string
	Handler Handler

	// If UserGuarder is set, the Username/password auth mode will be enabled
	UserGuarder UserGuarder
}

func (s *Server)Serve(listener net.Listener)(err error){
	for {
		var conn net.Conn
		if conn, err = listener.Accept(); err != nil {
			return
		}
		go s.Handle(conn)
	}
}

func Serve(listener net.Listener, handler Handler)(err error){
	server := &Server{
		Handler: handler,
	}
	return server.Serve(listener)
}

func (s *Server)ListenAndServe()(err error){
	addr := s.Addr
	if addr == "" {
		addr = DefaultSocksPort
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}
	return s.Serve(listener)
}

func ListenAndServe(addr string, handler Handler)(err error){
	server := &Server{
		Addr: addr,
		Handler: handler,
	}
	return server.ListenAndServe()
}

func (s *Server)tryAuth(c *conn, auth AuthType)(ok bool, err error){
	// TODO: add custom auths
	switch auth {
	case NoAuth:
		if _, err = c.Write([]byte{Socks5Version, auth}); err != nil {
			return
		}
		return true, nil
	case UserAuth:
		if s.UserGuarder == nil {
			return false, nil
		}
		var b byte
		if b, err = c.ReadByte(); err != nil {
			return
		}
		if b != 0x01 {
			return true, ErrUnsupportVersion
		}
		if b, err = c.ReadByte(); err != nil {
			return
		}
		id := make([]byte, b)
		if _, err = io.ReadFull(c, id); err != nil {
			return
		}
		if b, err = c.ReadByte(); err != nil {
			return
		}
		pw := make([]byte, b)
		if _, err = io.ReadFull(c, pw); err != nil {
			return
		}
		if !s.UserGuarder.Auth((string)(id), (string)(pw)) {
			if _, err = c.Write([]byte{0x01, 0xff}); err != nil {
				return
			}
			return true, ErrUnauthorized
		}
		if _, err = c.Write([]byte{0x01, 0x00}); err != nil {
			return
		}
		return true, nil
	}
	return false, nil
}

func (s *Server)Handle(netconn net.Conn)(err error){
	defer netconn.Close()

	// begin handshake & auth
	c := &conn{
		Conn: netconn,
	}
	var auths []AuthType
	if auths, err = c.readHandShake(); err != nil {
		return
	}
	ok := false
	for _, auth := range auths {
		if ok, err = s.tryAuth(c, auth); err != nil {
			return
		}
		if ok {
			break
		}
	}
	if !ok {
		if _, err = c.Write([]byte{Socks5Version, noAcceptableAuth}); err != nil {
			return
		}
		return ErrNoAcceptableAuth
	}

	// begin request
	cmd, addr, err := c.readRequest()
	if err != nil {
		return c.sendErrorResponse(err)
	}

	switch cmd {
	case CmdDialTCP:
		var conn net.Conn
		if conn, err = s.Handler.DialTCP(addr); err != nil {
			return c.sendErrorResponse(err)
		}
		if err = c.sendResponse(StatusGranted, nil); err != nil {
			return
		}

		go io.Copy(conn, c)
		_, err = io.Copy(c, conn)
		return
	case CmdBindTCP:
		var listener net.Listener
		if listener, err = s.Handler.BindTCP(addr); err != nil {
			return c.sendErrorResponse(err)
		}
		var ladr *AddrPort
		if ladr, err = AddrPortFromNetAddr(listener.Addr()); err != nil {
			// should never reach
			return
		}
		if err = c.sendResponse(StatusGranted, ladr); err != nil {
			return
		}
		var conn net.Conn
		if conn, err = listener.Accept(); err != nil {
			return c.sendErrorResponse(err)
		}
		var laddr *AddrPort
		if laddr, err = AddrPortFromNetAddr(conn.LocalAddr()); err != nil {
			// should never reach
			return
		}
		if err = c.sendResponse(StatusGranted, laddr); err != nil {
			return
		}

		go io.Copy(conn, c)
		_, err = io.Copy(c, conn)
		return
	case CmdForwardUDP: // TODO
		break

		var conn net.PacketConn
		if conn, err = s.Handler.BindUDP(addr); err != nil {
			return c.sendErrorResponse(err)
		}

		var listenIP net.IP
		switch addr.Type {
		case IPv4Addr:
			listenIP = net.IPv4(127, 0, 0, 1)
		case IPv6Addr:
			listenIP = net.IPv6loopback
		default:
			listenIP = net.IPv4(127, 0, 0, 1)
		}
		var lc *net.UDPConn
		if lc, err = net.ListenUDP("udp", &net.UDPAddr{IP: listenIP}); err != nil {
			return c.sendResponse(StatusCommandUnsupported, nil)
		}
		defer lc.Close()

		var laddr *AddrPort
		if laddr, err = AddrPortFromNetAddr(lc.LocalAddr()); err != nil {
			// should never reach
			return
		}
		if err = c.sendResponse(StatusGranted, laddr); err != nil {
			return
		}

		go forwardPacket(conn, lc, nil) // TODO
		go forwardPacket(lc, conn, nil)

		readUntilClosed(c)
		return
	}
	if err = c.sendResponse(StatusCommandUnsupported, nil); err != nil {
		return
	}
	return
}
