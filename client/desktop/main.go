
// Copyright (C) 2023  Kevin Z <zyxkad@gmail.com>
package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kmcsr/wpn"
	"github.com/kmcsr/wpn/l2tp"
)

func main(){
	RESTART:
	done := make(chan struct{}, 0)
	client := wpn.NewClient(config.Server)
	client.Logger = loger
	{
		ping, err := client.Ping()
		if err != nil {
			// loger.Fatalf("Cannot ping the server: %v", err)
		}
		loger.Infof("Connected to the server: ping=%v", ping)
	}
	go func(){
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Second * 10):
				ping, err := client.Ping()
				if err != nil {
					loger.Errorf("Cannot ping the server: %v", err)
				}else{
					loger.Debugf("Ping=%v", ping)
				}
			}
		}
	}()
	server := &l2tp.Server{
		// Addr: config.SocksAddr,
		Logger: loger,
	}

	go func(){
		defer close(done)
		loger.Infof("Starting L2TP server at %q", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			loger.Fatalf("Error when running L2TP server: %v", err)
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	WAIT:
	select {
	case s := <-sigs:
		loger.Infof("Got signal %s", s.String())
		if s == syscall.SIGHUP {
			ncfg, err := loadConfig()
			if err != nil {
				loger.Errorf("Cannot load config: %v", err)
				goto WAIT
			}
			config = ncfg
			goto RESTART
		}
		timeoutCtx, _ := context.WithTimeout(context.Background(), 16 * time.Second)
		loger.Warn("Closing server...")
		server.Shutdown(timeoutCtx)
	case <-done:
		return
	}
}
