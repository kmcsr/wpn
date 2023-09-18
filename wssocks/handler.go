
package wssocks

import (
	"context"
	"net"

	"github.com/kmcsr/wpn"
	"github.com/kmcsr/wpn/socks5"
)

type Handler struct {
	Client *wpn.Client
}

var _ socks5.Handler = (*Handler)(nil)

func (h *Handler)DialTCP(ctx context.Context, addr *socks5.AddrPort)(net.Conn, error){
	network := "tcp"
	switch addr.Type {
	case socks5.IPv4Addr:
		network = "tcp4"
	case socks5.IPv6Addr:
		network = "tcp6"
	}
	return h.Client.DialContext(ctx, network, addr.String())
}

func (h *Handler)BindTCP(ctx context.Context, addr *socks5.AddrPort)(net.Listener, error){
	return nil, socks5.ErrUnsupportCommand
}

func (h *Handler)BindUDP(ctx context.Context, addr *socks5.AddrPort)(net.PacketConn, error){
	network := "udp"
	switch addr.Type {
	case socks5.IPv4Addr:
		network = "udp4"
	case socks5.IPv6Addr:
		network = "udp6"
	}
	return h.Client.ListenPacketContext(ctx, network, addr.String())
}

