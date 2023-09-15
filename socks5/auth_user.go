
package socks5

import (
	"sync"
	"time"
)

type UserGuarder interface {
	Auth(username string, password string)(ok bool)
}

type MemoryUserGuarder struct {
	mux  sync.RWMutex
	data map[string]string
}

var _ UserGuarder = (*MemoryUserGuarder)(nil)

func NewMemoryUserGuarder()(*MemoryUserGuarder){
	return &MemoryUserGuarder{
		data: make(map[string]string),
	}
}

func NewMemoryUserGuarderFromMap(m map[string]string)(*MemoryUserGuarder){
	if m == nil {
		panic("map cannot be nil")
	}
	return &MemoryUserGuarder{
		data: m,
	}
}

func (g *MemoryUserGuarder)Auth(username string, password string)(ok bool){
	protector := time.After(time.Microsecond * 100) // to avoid username and password guess
	defer func(){ <-protector }()

	g.mux.RLock()
	pswd, ok := g.data[username]
	g.mux.RUnlock()
	if !ok {
		return false
	}
	return pswd == password
}

func (g *MemoryUserGuarder)ListUsers()(users []string){
	g.mux.RLock()
	defer g.mux.RUnlock()

	users = make([]string, 0, len(g.data))
	for user, _ := range g.data {
		users = append(users, user)
	}
	return
}

func (g *MemoryUserGuarder)SetUser(username string, password string){
	g.mux.Lock()
	defer g.mux.Unlock()

	g.data[username] = password
}

func (g *MemoryUserGuarder)DelUser(username string){
	g.mux.Lock()
	defer g.mux.Unlock()

	delete(g.data, username)
}
