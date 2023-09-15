
package socks5

import (
	"errors"
	"io"
	"net"
	"strings"
)

const (
	Socks5Version = 0x05
)

type conn struct {
	net.Conn

	version byte
}

type AuthType = byte

const (
	NoAuth     AuthType = 0x00
	// [GSSAPI](https://en.wikipedia.org/wiki/GSSAPI) ([RFC 1961](https://datatracker.ietf.org/doc/html/rfc1961))
	GSSAPIAuth AuthType = 0x01
	// Username/password ([RFC 1929](https://datatracker.ietf.org/doc/html/rfc1929))
	UserAuth   AuthType = 0x02

	// When no acceptable methods were offered, send from server only
	noAcceptableAuth AuthType = 0xff
)

func (c *conn)ReadByte()(b byte, err error){
	return readByte(c.Conn)
}

func (c *conn)readHandShake()(auths []AuthType, err error){
	if c.version, err = c.ReadByte(); err != nil {
		return
	}
	if c.version != Socks5Version {
		return nil, &UnsupportVersionError{c.version}
	}
	var l byte
	if l, err = c.ReadByte(); err != nil {
		return
	}
	auths = make([]AuthType, l)
	if _, err = io.ReadFull(c, auths); err != nil {
		return nil, err
	}
	return
}

func (c *conn)readAndVaildVersion()(err error){
	var v byte
	if v, err = c.ReadByte(); err != nil {
		return
	}
	if v != c.version {
		return ErrVersionChanged
	}
	return
}

func (c *conn)readRequest()(cmd byte, addr *AddrPort, err error){
	if err = c.readAndVaildVersion(); err != nil {
		return
	}
	if cmd, err = c.ReadByte(); err != nil {
		return
	}
	if _, err = c.ReadByte(); err != nil { // read the reserved byte
		return
	}

	addr = new(AddrPort)
	if _, err = addr.ReadFrom(c); err != nil {
		return
	}
	return
}

func (c *conn)sendResponse(status StatusCode, addr *AddrPort)(err error){
	if _, err = c.Write([]byte{c.version, (byte)(status), 0x00}); err != nil {
		return
	}
	if _, err = addr.WriteTo(c); err != nil {
		return
	}
	return
}

func (c *conn)sendErrorResponse(err error)(error){
	status := StatusFailed
	if errors.Is(err, ErrUnsupportVersion) {
		status = StatusAddrTypeUnsupported
	}else if errors.Is(err, ErrUnsupportCommand){
		status = StatusCommandUnsupported
	}else if errors.Is(err, ErrTargetBlocked) {
		status = StatusBlocked
	}else{
		msg := err.Error()
		if strings.Contains(msg, "refused") {
			status = StatusRefused
		}else if strings.Contains(msg, "unreachable") {
			if strings.Contains(msg, "network") {
				status = StatusNetworkUnreachable
			}else{
				status = StatusHostUnreachable
			}
		}else if strings.Contains(msg, "blocked") || strings.Contains(msg, "not allowed") {
			status = StatusBlocked
		}
	}
	return c.sendResponse(status, nil)
}
