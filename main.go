package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/eyesore/wshandler"
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
	setupLogger()
	flag.Usage = usage

	setOptions()
	flag.Parse()
	if args := flag.Args(); len(args) > 1 && (args[1] == "help" || args[1] == "-h") {
		flag.Usage()
		return
	}

	wshandler.SetDebug(false)

	if !Config().DisableWebsocket {
		h := wshandler.NewConnector(NewWorker)
		h.AllowAnyOrigin()

		http.Handle("/", h)
		websocketPort := ":" + Config().WebsocketPort
		err := http.ListenAndServe(websocketPort, nil)
		if err != nil {
			zap.S().Fatal("Failed to start server")
		}
	}
	if !Config().DisableStratum {
		// start stratum server
	}
}
