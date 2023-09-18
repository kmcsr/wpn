
package wpn

import (
	"sync"
)

// a thread safty connection pool
type connPool struct {
	mux   sync.RWMutex
	data []*Conn
}

func (s *connPool)Len()(int){
	s.mux.RLock()
	defer s.mux.RUnlock()

	return len(s.data)
}

func (s *connPool)Clear()(conns []*Conn){
	s.mux.Lock()
	defer s.mux.Unlock()
	conns = s.data
	s.data = nil
	return
}

func (s *connPool)Put(c *Conn){
	s.mux.Lock()
	defer s.mux.Unlock()

	s.data = append(s.data, c)
}

// This is a thread safty Put + Remove
func (s *connPool)PutFrom(c *Conn, src *connPool){
	s.mux.Lock()
	defer s.mux.Unlock()
	src.mux.Lock()
	defer src.mux.Unlock()

	if !src.remove(c) {
		panic("wpn: Conn is not in the source pool")
	}
	s.data = append(s.data, c)
}

func (s *connPool)Remove(c *Conn)(ok bool){
	s.mux.Lock()
	defer s.mux.Unlock()
	return s.remove(c)
}

func (s *connPool)remove(c *Conn)(ok bool){
	last := len(s.data) - 1
	for i, v := range s.data {
		if v == c {
			if i != last { // i < last
				s.data[i] = s.data[last]
			}
			s.data = s.data[:last]
			return true
		}
	}
	return false
}

func (s *connPool)TakeOr(newer func()(*Conn))(c *Conn){
	s.mux.Lock()
	defer s.mux.Unlock()

	last := len(s.data) - 1
	if last < 0 {
		if newer == nil {
			return nil
		}
		return newer()
	}
	s.data, c = s.data[:last], s.data[last]
	return
}
