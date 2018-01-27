package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	ews "github.com/eyesore/ws"
	"github.com/trey-jones/xmrwasp/config"
	"github.com/trey-jones/xmrwasp/logger"
	"github.com/trey-jones/xmrwasp/tcp"
	"github.com/trey-jones/xmrwasp/ws"
)

var (
	version = "1.0.0"

	// cmd line options
	configFile *string
)

func printWelcomeMessage() {
	logger.Get().Println("************************************************************************")
	logger.Get().Printf("*    XMR Web and Stratum Proxy \t\t\t\t v%s \n", version)
	if !config.Get().DisableWebsocket {
		port := config.Get().WebsocketPort
		logger.Get().Printf("*    Accepting Websocket Connections on port: \t\t %v\n", port)
	}
	if !config.Get().DisableTCP {
		port := config.Get().StratumPort
		logger.Get().Printf("*    Accepting TCP Connections on port: \t\t\t\t %v\n", port)
	}
	statInterval := config.Get().StatInterval
	logger.Get().Printf("*    Printing stats every: \t\t\t\t %v seconds\n", statInterval)
	logger.Get().Println("************************************************************************")
}

func usage() {
	fmt.Printf("Usage: %s [-c CONFIG_PATH] \n", os.Args[0])
	flag.PrintDefaults()
}

func setOptions() {
	configFile = flag.String("c", "", "JSON file from which to read configuration values")
	flag.Parse()

	config.File = *configFile
}

func setupLogger() {
	lc := &logger.Config{W: nil}
	c := config.Get()
	if c.Debug {
		lc.Level = logger.Debug
	}
	if c.LogFile != "" {
		f, err := os.Create(c.LogFile)
		if err != nil {
			log.Fatal("could not open log file for writing: ", err)
		}
		lc.W = f
	}
	if c.DiscardLog {
		lc.Discard = true
	}
	logger.Configure(lc)
	logger.Get().Debugln("logger is configured")
}

func main() {
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

	if config.Get().DisableWebsocket && config.Get().DisableTCP {
		logger.Get().Fatal("No servers configured for listening.  Bye!")
	}
	if !config.Get().DisableWebsocket {
		go ws.StartServer()
	}
	if !config.Get().DisableTCP {
		go tcp.StartServer()
	}

	printWelcomeMessage()

	<-holdOpen
}
