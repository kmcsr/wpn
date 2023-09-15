
package main

import (
	"net/http"

	"github.com/kmcsr/wpn"
)

func main(){
	server := wpn.NewServer()
	http.ListenAndServe("127.0.0.1:8095", server)
}
