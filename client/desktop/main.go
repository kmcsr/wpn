
package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/kmcsr/wpn"
)

func test(hc *http.Client){
	start := time.Now()
	res, err := hc.Get("https://google.com")
	if err != nil {
		println("==> error when getting:", err.Error(), res)
		return
	}
	defer res.Body.Close()
	buf, err := io.ReadAll(res.Body)
	used := time.Since(start)
	fmt.Println(used, err)
	fmt.Println(len(buf))
}

func main(){
	cli := wpn.NewClient("ws://127.0.0.1:8095")
	test(http.DefaultClient)
	hc := &http.Client{
		Transport: &http.Transport{
			DialContext: cli.DialContext,
		},
	}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(){
				defer wg.Done()
				test(hc)
			}()
		}
		wg.Wait()
	}
	println("all done")
	time.Sleep(time.Second * 30)
}
