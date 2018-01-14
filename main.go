package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/eyesore/wshandler"
	"github.com/trey-jones/xmrwasp/config"
	"github.com/trey-jones/xmrwasp/tcp"
	"github.com/trey-jones/xmrwasp/ws"
	"go.uber.org/zap"
)

var (
	// cmd line options
	configFile *string
)

func usage() {
	fmt.Printf("Usage: %s [CONFIG_PATH] \n", os.Args[0])
	flag.PrintDefaults()
}

func setOptions() {
	configFile = flag.String("c", "", "JSON file from which to read configuration values")
	flag.Parse()

	config.File = *configFile
}

func setupLogger() {
	var l *zap.Logger
	if config.Get().Debug {
		l, _ = zap.NewDevelopment()
	} else {
		l, _ = zap.NewProduction()
	}
	zap.ReplaceGlobals(l)
}

func main() {
	// TODO define logging interface
	setOptions()
	setupLogger()

	flag.Usage = usage

	flag.Parse()
	if args := flag.Args(); len(args) > 1 && (args[1] == "help" || args[1] == "-h") {
		flag.Usage()
		return
	}
	config.File = *configFile

	wshandler.SetDebug(false)
	holdOpen := make(chan bool, 1)

	if !config.Get().DisableWebsocket {
		go ws.StartServer(zap.S())
	}
	if !config.Get().DisableTCP {
		go tcp.StartServer(zap.S())
	}

	if !config.Get().Background {
		<-holdOpen
	}
	zap.S().Sync()
}
