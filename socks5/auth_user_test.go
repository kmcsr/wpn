
package socks5_test

import (
	"testing"
	"time"

	"github.com/kmcsr/wpn/socks5"
)

func TestMemoryGuarder(t *testing.T){
	g := socks5.NewMemoryUserGuarder()
	g.SetUser("user", "password")
	getUsedTime := func(user string, pswd string)(time.Duration){
		before := time.Now()
		g.Auth(user, pswd)
		return time.Since(before)
	}

	testDatas := [][2]string{
		{"not exist user", "any password"},
		{"user", "not vaild password"},
		{"user", "password"},
		{"user", "paccword"},
	}
	for _, d := range testDatas {
		t.Logf("user=%q passwd=%q used=%v", d[0], d[1], getUsedTime(d[0], d[1]))
	}
}
