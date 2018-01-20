package logger

import (
	"io"
	"log"
	"os"
	"sync"
)

const (
	// Info and Debug are the two possible logging levels
	// The only difference is the Debug method does nothing on info level
	Info = iota
	Debug
)

var (
	// defaults
	config = &Config{
		os.Stdout,
		log.LstdFlags,
		Info,
	}

	// global logger singleton
	instance      *Logger
	instantiation = sync.Once{}
)

// Config allows selection of logger output, content and level (debug!)
type Config struct {
	W           io.Writer
	Flag, Level int
}

// Logger wraps the standard logger and adds a debug level
type Logger struct {
	*log.Logger
	Level int
}

// Configure sets up the global logger.  This should be called from the main thread
// before the logger is created with Get
func Configure(c *Config) {
	config = c
}

// New makes a new logger with config.
func New(c *Config) *Logger {
	return &Logger{
		log.New(c.W, "[XMRWASP]", c.Flag),
		c.Level,
	}
}

// Get returns the global singleton logger
func Get() *Logger {
	instantiation.Do(func() {
		instance = New(config)
	})

	return instance
}

func (l *Logger) Debug(v ...interface{}) {
	if l.Level < Debug {
		return
	}
	l.Logger.Print(v)
}

func (l *Logger) Debugf(format string, v ...interface{}) {
	if l.Level < Debug {
		return
	}
	l.Logger.Printf(format, v)
}

func (l *Logger) Debugln(v ...interface{}) {
	if l.Level < Debug {
		return
	}

	l.Logger.Println(v)
}
