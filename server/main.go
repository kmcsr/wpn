
package main

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/kmcsr/wpn"
)

func loadBlockRules()(s *wpn.IPRuleSet){
	var fd io.ReadCloser
	fd, err := os.Open("block-rules.txt")
	if err != nil {
		if os.IsNotExist(err){
			loger.Warn("Block rule file is not exists, creating one ...")
			if err = os.WriteFile("block-rules.txt", ([]byte)(defaultBlockRuleContent), 0644); err != nil {
				loger.Fatalf("Cannot create block-rules.txt: %v", err)
			}
			fd = io.NopCloser(strings.NewReader(defaultBlockRuleContent))
		}else{
			loger.Fatalf("Cannot read block rules: %v", err)
		}
	}
	defer fd.Close()
	if s, err = wpn.ReadIPRuleSet(fd); err != nil {
		if es, ok := err.(interface{ Unwrap()([]error) }); ok {
			var buf strings.Builder
			buf.WriteString("Error when parsing block rules:\n")
			for _, e := range es.Unwrap() {
				buf.WriteByte('\t')
				buf.WriteString(e.Error())
				buf.WriteByte('\n')
			}
			loger.Error(buf.String())
		}else{
			loger.Fatalf("Cannot read block-rules: %v", err)
		}
	}
	return
}

func main(){
	blocks := loadBlockRules()
	server := wpn.NewServer()
	server.Logger = loger
	server.Blocks = blocks
	hsvr := &http.Server{
		Addr: config.Addr,
		Handler: server,
	}
	done := make(chan struct{}, 1)
	go func(){
		defer func(){
			done <- struct{}{}
		}()
		loger.Infof("Server start at %q", hsvr.Addr)
		if err := hsvr.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			loger.Fatalf("HTTP Server error: %v", err)
		}
	}()

	select {
	case <-done:
		return
	}
}
