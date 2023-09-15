
package main

import (
	"os"
	"encoding/json"
)

type Config struct {
	Server string `json:"server"` // The remote server url
	SocksAddr string `json:"socksAddr"` // The socks5 server address
}

var config = func()(*Config){
	cfg, err := loadConfig()
	if err != nil {
		loger.Fatalf("Cannot load config file: %v", err)
	}
	return cfg
}()

func loadConfig()(cfg *Config, err error){
	buf, err := os.ReadFile("config.json")
	if err != nil {
		if os.IsNotExist(err) {
			cfg = &Config{ // default config
				Server: "ws://127.0.0.1:8095",
				SocksAddr: "127.0.0.1:1080",
			}
			if buf, err = json.MarshalIndent(cfg, "", "  "); err != nil {
				return
			}
			if err = os.WriteFile("config.json", buf, 0644); err != nil {
				return
			}
			return
		}
		return
	}
	cfg = new(Config)
	if err = json.Unmarshal(buf, cfg); err != nil {
		return
	}
	return
}
