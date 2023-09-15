
package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/kmcsr/go-logger"
	logrusl "github.com/kmcsr/go-logger/logrus"
)

var DEBUG = true

var loger = initLogger()

func initLogger()(loger logger.Logger){
	loger = logrusl.New()
	logrusl.Unwrap(loger).SetFormatter(&logrus.TextFormatter{
		TimestampFormat: "2006-01-02 15:04:05.000",
		FullTimestamp: true,
	})
	if DEBUG {
		loger.SetLevel(logger.DebugLevel)
	}else{
		loger.SetLevel(logger.InfoLevel)
		_, err := logger.OutputToFile(loger, "/var/log/wpn/server.log", os.Stderr)
		if err != nil {
			loger.SetOutput(os.Stderr)
			loger.Fatalf("Cannot open log file: %v", err)
		}
	}
	return
}
