
package wpn_test

import (
	"testing"
	"net/netip"

	. "github.com/kmcsr/wpn"
)

func TestIPRule(t *testing.T){
	type testT struct {
		rule string
		addr string
		match bool
	}
	datas := []testT{
		{"127.0.0.1", "127.0.0.0", false},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.0.0.1/8", "127.0.0.0", true},
		{"127.0.0.1/8", "127.1.3.2", true},
		{"127.0.0.1/8", "128.0.0.0", false},
		{"127.0.0.1/8", "::ff:127.0.0.1", false},
		{"127.0.0.1/8", "::ffff:127.0.0.1", true},
		{"127.0.0.1/8", "::ffff:127.0.0.127", true},
		{"127.0.0.1/8", "::ffff:128.0.0.1", false},
		{"127.0.0.1/8", "ff::ffff:127.0.0.1", false},
		{"!127.0.0.1/8", "127.0.0.0", false},
		{"!127.0.0.1/8", "127.0.0.1", false},
		{"!127.0.0.1/8", "128.0.0.1", true},
		{"!127.0.0.1/8", "::ffff:127.0.0.1", false},
		{"::ffff:127.0.0.1", "127.0.0.1", false},
	}
	for _, v := range datas {
		r, err := ParseIPRule(v.rule)
		if err != nil {
			t.Fatalf("Cannot parse rule %q: %v", v.rule, err)
		}
		if contains := r.Contains(netip.MustParseAddr(v.addr)); contains != v.match {
			t.Errorf("Wrong act of rule %q, contains(%q) = %v, but expected %v", v.rule, v.addr, contains, v.match)
		}
	}
}

