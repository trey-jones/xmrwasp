package main

import (
	"flag"
	"fmt"
	"os"

	ews "github.com/eyesore/ws"
	"github.com/trey-jones/xmrwasp/config"
	"github.com/trey-jones/xmrwasp/tcp"
	"github.com/trey-jones/xmrwasp/ws"
	"go.uber.org/zap"
)

var (
	version = "1.0.0"

	// cmd line options
	configFile *string
)

func printWelcomeMessage() {
	fmt.Println("************************************************************************")
	fmt.Printf("*    XMR Web and Stratum Proxy \t\t\t\t v%s \n", version)
	if !config.Get().DisableWebsocket {
		port := config.Get().WebsocketPort
		fmt.Printf("*    Accepting Websocket Connections on port: \t\t %s\n", port)
	}
	if !config.Get().DisableTCP {
		port := config.Get().StratumPort
		fmt.Printf("*    Accepting TCP Connections on port: \t\t\t\t %s\n", port)
	}
	statInterval := config.Get().StatInterval
	fmt.Printf("*    Printing stats every: \t\t\t\t %v seconds\n", statInterval)
	fmt.Println("************************************************************************")
}

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

	ews.SetDebug(false)
	holdOpen := make(chan bool, 1)

	if !config.Get().DisableWebsocket {
		go ws.StartServer(zap.S())
	}
	if !config.Get().DisableTCP {
		go tcp.StartServer(zap.S())
	}

	printWelcomeMessage()

	if !config.Get().Background {
		// TODO - this won't do it
		<-holdOpen
	}
	zap.S().Sync()
}
