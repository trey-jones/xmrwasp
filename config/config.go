package config

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/errors"
)

var (
	// File specifies a file from which to read the config
	// If empty, config will be read from the environment
	File string

	instance      *Config
	oneTimeConfig = sync.Once{}

	// ErrMissingRequiredConfig indicates that required configuration has not been set.
	ErrMissingRequiredConfig = errors.New("missing required config")
)

// Config holds the global application configuration.
type Config struct {
	Debug bool `envconfig:"debug" json:"debug"`

	DisableWebsocket bool   `envconfig:"noweb" json:"noweb"`
	DisableTCP       bool   `envconfig:"notcp" default:"true" json:"notcp"`
	WebsocketPort    string `envconfig:"wsport" default:"8080" json:"wsport"`
	StratumPort      string `envconfig:"strport" default:"1111" json:"strport"`

	SecureWebsocket bool   `envconfig:"wss" json:"wss"`
	CertFile        string `envconfig:"tlscert" json:"tlscert"`
	KeyFile         string `envconfig:"tlskey" json:"tlskey"`
	// TODO also support TLS for stratum connections

	// TODO multiple pools for fallback
	PoolAddr     string `envconfig:"url" required:"true" json:"url"`
	PoolLogin    string `envconfig:"login" required:"true" json:"login"`
	PoolPassword string `envconfig:"password" required:"true" json:"password"`

	StatInterval int `envconfig:"stats" default:"60" json:"stats"`

	DonateLevel int `envconfig:"donate" default:"2" json:"donate"`

	// LogFile and DiscardLog are mutually exclusive - logfile will be used if present
	LogFile    string `envconfig:"log" json:"log"`
	DiscardLog bool   `envconfig:"nolog" json:"nolog"`

	// not yet implemented
	Background bool `envconfig:"background" json:"background"`
}

// This only needs to be run if read from JSON
func validateAndSetDefaults(c *Config) error {
	// TODO cleanup?
	val := reflect.ValueOf(c)
	refType := reflect.TypeOf(c)
	for i := 0; i < val.Elem().NumField(); i++ {
		field := val.Elem().Field(i)
		fieldType := field.Type()
		defaultValue := refType.Elem().Field(i).Tag.Get("default")
		if defaultValue != "" {
			valueType := fieldType.Kind()
			switch valueType {
			case reflect.String:
				if field.String() == "" && field.CanSet() {
					field.SetString(defaultValue)
				}
			case reflect.Int:
				intVal, err := strconv.Atoi(defaultValue)
				if err != nil {
					log.Fatal("Unable to convert default value to int: ", defaultValue)
				}
				if field.Int() == 0 && field.CanSet() {
					field.SetInt(int64(intVal))
				}
			case reflect.Bool:
				if field.CanSet() {
					v, err := strconv.ParseBool(defaultValue)
					if err != nil {
						return errors.Wrap(err, "Unable to parse bool value for"+defaultValue)
					}
					field.SetBool(v)
				}
			default:
				log.Println("Unexpected type found in config.  Skipping: ", field)
			}
		}
		if _, ok := refType.Elem().Field(i).Tag.Lookup("required"); ok && field.String() == "" {
			fmt.Println("Missing required field in config: ", refType.Elem().Field(i).Name)
			return ErrMissingRequiredConfig
		}
	}

	return nil
}

func configFromEnv() error {
	cfg := Config{}
	err := envconfig.Process("xmrwasp", &cfg)
	if err != nil {
		if strings.Contains(err.Error(), "required") {
			// very loose condition
			return ErrMissingRequiredConfig
		}
		return err
	}
	instance = &cfg
	return nil
}

func configFromFile(r io.Reader) error {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrap(err, "Failed to read config file.")
	}

	cfg := Config{}
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return errors.Wrap(err, "Failed to parse JSON.")
	}
	err = validateAndSetDefaults(&cfg)
	if err != nil {
		return err
	}

	instance = &cfg
	return nil
}

// Get returns the global configuration singleton.
func Get() *Config {
	var err error
	oneTimeConfig.Do(func() {
		if File != "" {
			var f io.Reader
			f, err = os.Open(File)
			if err != nil {
				log.Fatal("open config file failed: ", err)
				return
			}
			err = configFromFile(f)
		} else {
			// try to read config from environment
			err = configFromEnv()
		}
	})
	if err != nil {
		log.Fatal("Unable to load config: ", err)
	}
	return instance
}
