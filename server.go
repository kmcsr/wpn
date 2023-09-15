
package wpn

import (
	"log"
	"net/http"
	"sync/atomic"

	"nhooyr.io/websocket"
)

type Server struct {
	WebsocketOpts *websocket.AcceptOptions

	count atomic.Int32
}

var _ http.Handler = (*Server)(nil)

func NewServer()(s *Server){
	s = &Server{}
	return
}

func (s *Server)ServeHTTP(rw http.ResponseWriter, req *http.Request){
	ws, err := websocket.Accept(rw, req, s.WebsocketOpts)
	if err != nil {
		return
	}
	s.count.Add(1)
	defer s.count.Add(-1)
	conn := WrapConn(req.Context(), ws)
	defer conn.Close()
	if err = conn.Handle(); err != nil {
		log.Printf("handle error: %s; %v\n", req.RemoteAddr, err.Error())
	}
}
