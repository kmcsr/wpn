
package wpn

import (
	"net"
	"strings"
	"sync"
)

type stringAddr struct {
	network string
	addr string
}

var _ net.Addr = (*stringAddr)(nil)

func (a *stringAddr)Network()(string){ return a.network }
func (a *stringAddr)String()(string){ return a.addr }

type ConnPipe struct {
	net.Conn
	localAddr, remoteAddr net.Addr
	closer func()
	closed chan struct{}
}

var _ net.Conn = (*ConnPipe)(nil)

func newConnPipe()(a, b *ConnPipe){
	a = new(ConnPipe)
	b = new(ConnPipe)
	ch := make(chan struct{}, 0)
	closer := sync.OnceFunc(func(){
		close(ch)
	})
	a.closer, b.closer = closer, closer
	a.closed, b.closed = ch, ch
	a.Conn, b.Conn = net.Pipe()
	return
}

func (p *ConnPipe)Close()(error){
	p.Conn.Close()
	p.closer()
	return nil
}

func (p *ConnPipe)AfterClose()(<-chan struct{}){
	return p.closed
}

func (p *ConnPipe)LocalAddr()(net.Addr){
	return p.localAddr
}

func (p *ConnPipe)RemoteAddr()(net.Addr){
	return p.remoteAddr
}

func split(str string, b byte)(l, r string){
	i := strings.IndexByte(str, b)
	if i >= 0 {
		return str[:i], str[i + 1:]
	}
	return str, ""
}
