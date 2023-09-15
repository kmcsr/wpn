
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
	return h.Client.DialContext(ctx, "tcp", addr.String())
}

func (h *Handler)BindTCP(ctx context.Context, addr *socks5.AddrPort)(net.Listener, error){
	return nil, socks5.ErrUnsupportCommand
}

func (h *Handler)BindUDP(ctx context.Context, addr *socks5.AddrPort)(net.PacketConn, error){
	return nil, socks5.ErrUnsupportCommand
}

