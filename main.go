package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/eyesore/wshandler"
	"github.com/trey-jones/xmrwasp/config"
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
}

func setupLogger() {
	// TODO handle this remote possibility
	l, _ := zap.NewDevelopment()
	zap.ReplaceGlobals(l)
}

func main() {
	// TODO define logging interface
	setupLogger()
	setOptions()

	flag.Usage = usage

	flag.Parse()
	if args := flag.Args(); len(args) > 1 && (args[1] == "help" || args[1] == "-h") {
		flag.Usage()
		return
	}
	config.File = *configFile

	wshandler.SetDebug(false)

	go ws.StartServer(zap.S())

	if !config.Get().DisableStratum {
		// start stratum server
	}
}
