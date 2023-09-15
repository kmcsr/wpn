
package socks5

import (
	"io"
	"net"
	"sync"
)

var bufPool = sync.Pool{
	New: func()(any){
		buf := make([]byte, 65536)
		return &buf
	},
}

func readByte(r io.Reader)(b byte, err error){
	if br, ok := r.(io.ByteReader); ok {
		return br.ReadByte()
	}
	var buf [1]byte
	if _, err = r.Read(buf[:]); err != nil {
		return
	}
	return buf[0], nil
}

func readUntilClosed(r io.Reader){
	var buf [1]byte
	_, _ = r.Read(buf[:])
}

func forwardPacket(dst, src net.PacketConn, target net.Addr)(err error){
	buf0 := bufPool.Get().(*[]byte)
	defer bufPool.Put(buf0)
	buf := *buf0
	var n int
	for {
		n, _, err = src.ReadFrom(buf)
		if err != nil {
			return
		}
		dst.WriteTo(buf[:n], target)
	}
}
