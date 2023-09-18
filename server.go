
package wpn

import (
	"context"
	"errors"
	"strings"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strconv"

	"nhooyr.io/websocket"
	"github.com/kmcsr/go-logger"
	logrusl "github.com/kmcsr/go-logger/logrus"
)

var (
	ErrIPBlocked = errors.New("IP is blocked")
	ErrNetworkUnreachable = errors.New("Network unreachable") // network is not support
	ErrHostUnreachable = errors.New("Host unreachable")
)

type Server struct {
	Logger logger.Logger

	WebsocketOpts  *websocket.AcceptOptions
	Resolver       *net.Resolver
	Dialer         Dialer
	PacketListener PacketListener

	Blocks *IPRuleSet
}

var _ http.Handler = (*Server)(nil)

func NewServer()(s *Server){
	s = &Server{
		Logger: logrusl.Logger,
	}
	return
}

func getBaseProtococl(protocol string)(string){
	if i := strings.IndexByte(protocol, ':'); i >= 0 {
		protocol = protocol[:i]
	}
	return protocol
}

func getLookupProtocol(protocol string)(string){
	if len(protocol) == 0 {
		return "ip"
	}
	switch protocol[len(protocol) - 1] {
	case '4':
		return "ip4"
	case '6':
		return "ip6"
	}
	return "ip"
}

func (s *Server)dialer(ctx context.Context, protocol string, target string)(conn net.Conn, err error){
	host, port0, err := net.SplitHostPort(target)
	if err != nil {
		return
	}
	port, err := strconv.ParseUint(port0, 10, 16)
	if err != nil {
		return
	}

	var ips []netip.Addr
	switch baseP := getBaseProtococl(protocol); baseP {
	case "tcp", "tcp4", "tcp6":
		resolver := s.Resolver
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		if ips, err = resolver.LookupNetIP(ctx, getLookupProtocol(baseP), host); err != nil {
			return
		}
	default:
		return nil, ErrNetworkUnreachable
	}
	dialer := s.Dialer
	if dialer == nil {
		dialer = dialContext
	}
	for _, v := range ips {
		if s.Blocks.Contains(v) {
			err = ErrIPBlocked
		}else{
			if conn, err = dialer(ctx, protocol, netip.AddrPortFrom(v, (uint16)(port)).String()); err == nil {
				return
			}
		}
	}
	if err == nil {
		err = ErrHostUnreachable
	}
	return
}

func (s *Server)pktListener(ctx context.Context, protocol string, target string)(conn net.PacketConn, err error){
	host, port0, err := net.SplitHostPort(target)
	if err != nil {
		return
	}
	port, err := strconv.ParseUint(port0, 10, 16)
	if err != nil {
		return
	}

	var ips []netip.Addr
	switch baseP := getBaseProtococl(protocol); baseP {
	case "ip", "ip4", "ip6", "udp", "udp4", "udp6":
		resolver := s.Resolver
		if resolver == nil {
			resolver = net.DefaultResolver
		}
		if ips, err = resolver.LookupNetIP(ctx, getLookupProtocol(baseP), host); err != nil {
			return
		}
	default:
		return nil, ErrNetworkUnreachable
	}
	listener := s.PacketListener
	if listener == nil {
		listener = listenPacketContext
	}
	for _, v := range ips {
		if s.Blocks.Contains(v) {
			err = ErrIPBlocked
		}else{
			if conn, err = listener(ctx, protocol, netip.AddrPortFrom(v, (uint16)(port)).String()); err == nil {
				return
			}
		}
	}
	return
}

func (s *Server)ServeHTTP(rw http.ResponseWriter, req *http.Request){
	defer func(){
		err := recover()
		if err != nil {
			s.Logger.Errorf("wpn.Server: Error during serve http: %v\n%s", err, (string)(debug.Stack()))
		}
	}()

	ws, err := websocket.Accept(rw, req, s.WebsocketOpts)
	if err != nil {
		return
	}

	conn := WrapConn(req.Context(), ws)
	defer conn.Close()
	conn.Dialer = s.dialer
	conn.PacketListener = s.pktListener

	if err = conn.Handle(); err != nil {
		s.Logger.Errorf("wpn.Server: Handle error: %s; %v\n", req.RemoteAddr, err.Error())
	}
}
