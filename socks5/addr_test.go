
package socks5_test

import (
	"testing"
	"net"

	"github.com/kmcsr/wpn/socks5"
)

func TestAddrString(t *testing.T){
	type dataT struct{
		A *socks5.AddrPort
		S string
	}
	datas := []dataT{
		{(&socks5.AddrPort{}).SetIPv4(net.IPv4zero), "0.0.0.0:0"},
		{(&socks5.AddrPort{}).SetIPv6(net.IPv6zero), "[::]:0"},
		{(&socks5.AddrPort{Port: 234}).SetIPv4(net.IPv4(127, 0, 0, 1)), "127.0.0.1:234"},
		{(&socks5.AddrPort{Port: 7788}).SetIPv6(net.IPv6loopback), "[::1]:7788"},
		{(&socks5.AddrPort{}).SetDomain("example.com"), "example.com:0"},
		{(&socks5.AddrPort{Port: 3344}).SetDomain("example.com"), "example.com:3344"},
	}
	for _, d := range datas {
		s := d.A.String()
		if s != d.S {
			t.Errorf("Unexpected addr string %q, expect %q", s, d.S)
		}
	}
}
