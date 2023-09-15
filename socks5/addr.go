
package socks5

import (
	"io"
	"net"
	"net/netip"
	"strconv"
)

type AddrType byte
const (
	IPv4Addr   AddrType = 0x01
	DomainAddr AddrType = 0x03
	IPv6Addr   AddrType = 0x04
)

// AddrPort represents a TCP host:port pair
type AddrPort struct {
	Type AddrType
	Data []byte
	Port uint16
}

var _ io.ReaderFrom = (*AddrPort)(nil)
var _ io.WriterTo = (*AddrPort)(nil)

func AddrPortFromNetAddr(na net.Addr)(a *AddrPort, err error){
	if na == nil {
		return nil, nil
	}
	var addr netip.AddrPort
	addr, err = netip.ParseAddrPort(na.String())
	if err != nil {
		return
	}
	a = &AddrPort{
		Data: addr.Addr().AsSlice(),
		Port: addr.Port(),
	}
	switch len(a.Data) {
	case 4:
		a.Type = IPv4Addr
	case 16:
		a.Type = IPv6Addr
	default:
		a.Type = IPv4Addr
		a.Data = net.IPv4zero
	}
	return
}

func (a *AddrPort)SetIPv4(ip net.IP)(*AddrPort){
	a.Type = IPv4Addr
	a.Data = ([]byte)(ip.To4())
	if a.Data == nil {
		panic("ip is not a vaild IPv4 IP")
	}
	return a
}

func (a *AddrPort)SetDomain(domain string)(*AddrPort){
	if len(domain) > 0xff {
		panic("Domain size must less than 256")
	}
	a.Type = DomainAddr
	a.Data = ([]byte)(domain)
	return a
}

func (a *AddrPort)SetIPv6(ip net.IP)(*AddrPort){
	a.Type = IPv6Addr
	a.Data = ([]byte)(ip.To16())
	if a.Data == nil {
		panic("ip is not a vaild IPv6 IP")
	}
	return a
}

func (a *AddrPort)GetIPv4()(ip net.IP, ok bool){
	if a.Type != IPv4Addr {
		return
	}
	return ((net.IP)(a.Data)).To4(), true
}

func (a *AddrPort)GetDomain()(domain string, ok bool){
	if a.Type != DomainAddr {
		return
	}
	return (string)(a.Data), true
}

func (a *AddrPort)GetIPv6()(ip net.IP, ok bool){
	if a.Type != IPv6Addr {
		return
	}
	return ((net.IP)(a.Data)).To16(), true
}

func (a *AddrPort)String()(s string){
	switch a.Type {
	case IPv4Addr:
		return netip.AddrPortFrom(netip.AddrFrom4(([4]byte)(a.Data)), a.Port).String()
	case IPv6Addr:
		return netip.AddrPortFrom(netip.AddrFrom16(([16]byte)(a.Data)), a.Port).String()
	case DomainAddr:
		return (string)(a.Data) + ":" + strconv.Itoa((int)(a.Port))
	default:
		panic("Unknown addr type")
	}
}

func (a *AddrPort)ReadFrom(r io.Reader)(n int64, err error){
	var b byte
	if b, err = readByte(r); err != nil {
		return
	}
	n++
	a.Type = (AddrType)(b)
	switch a.Type {
	case IPv4Addr:
		a.Data = make([]byte, 4)
	case DomainAddr:
		if b, err = readByte(r); err != nil {
			return
		}
		n++
		a.Data = make([]byte, b)
	case IPv6Addr:
		a.Data = make([]byte, 16)
	default:
		return n, ErrUnsupportVersion
	}
	var n0 int
	n0, err = io.ReadFull(r, a.Data)
	n += (int64)(n0)
	if err != nil {
		return
	}
	var port [2]byte
	n0, err = io.ReadFull(r, port[:])
	n += (int64)(n0)
	if err != nil {
		return
	}
	a.Port = ((uint16)(port[0]) << 8) | (uint16)(port[1])
	return
}

func (a *AddrPort)Encode()(buf []byte, err error){
	if a == nil {
		return []byte{
			(byte)(IPv4Addr),
			0, 0, 0, 0,
			0, 0,
		}, nil
	}
	buf = make([]byte, 1, 1 + 16 + 2)
	buf[0] = (byte)(a.Type)
	switch a.Type {
	case IPv4Addr:
		if len(a.Data) != 4 {
			panic("socks5: Data's length for IPv4 must be 4")
		}
	case DomainAddr:
		l := len(a.Data)
		if l > 0xff {
			panic("socks5: domain's size is to large (must <= 0xff)")
		}
		buf = append(buf, (byte)(l))
	case IPv6Addr:
		if len(a.Data) != 16 {
			panic("socks5: Data's length for IPv6 must be 16")
		}
	default:
		return nil, ErrUnsupportVersion
	}
	buf = append(buf, a.Data...)
	buf = append(buf, (byte)(a.Port >> 8), (byte)(a.Port & 0xff))
	return
}

func (a *AddrPort)WriteTo(w io.Writer)(n int64, err error){
	var buf []byte
	if buf, err = a.Encode(); err != nil {
		return
	}
	var n0 int
	n0, err = w.Write(buf)
	n = (int64)(n0)
	return
}
