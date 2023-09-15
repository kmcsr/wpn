
package wssocks

import (
	"net"

	"github.com/kmcsr/wpn"
	"github.com/kmcsr/wpn/socks5"
)

type Handler struct {
	Client *wpn.Client
}

var _ socks5.Handler = (*Handler)(nil)

func (h *Handler)DialTCP(addr *socks5.AddrPort)(net.Conn, error){
	return h.Client.Dial("tcp", addr.String())
}

func (h *Handler)BindTCP(addr *socks5.AddrPort)(net.Listener, error){
	return nil, socks5.ErrUnsupportCommand
}

func (h *Handler)BindUDP(addr *socks5.AddrPort)(net.PacketConn, error){
	return nil, socks5.ErrUnsupportCommand
}

