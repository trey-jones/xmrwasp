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
	instantiation = sync.Once{}
)

// Config holds the global application configuration.
type Config struct {
	Debug bool `envconfig:"debug" json:"debug"`

	DisableWebsocket bool `envconfig:"noweb" json:"noweb"`
	DisableTCP       bool `envconfig:"notcp" default:"true" json:"notcp"`
	WebsocketPort    int  `envconfig:"wsport" default:"8080" json:"wsport"`
	StratumPort      int  `envconfig:"strport" default:"1111" json:"strport"`

	SecureWebsocket bool   `envconfig:"wss" json:"wss"`
	CertFile        string `envconfig:"tlscert" json:"tlscert"`
	KeyFile         string `envconfig:"tlskey" json:"tlskey"`
	// TODO also support TLS for stratum connections

	// TODO multiple pools for fallback
	PoolAddr     string `envconfig:"url" required:"true" json:"url"`
	PoolLogin    string `envconfig:"login" required:"true" json:"login"`
	PoolPassword string `envconfig:"password" required:"true" json:"password"`

	StatInterval int `envconfig:"stats" default:"60" json:"stats"`

	ShareValidation int `envconfig:"validateshares" json:"validateshares"`

	DonateLevel int `envconfig:"donate" default:"2" json:"donate"`

	// LogFile and DiscardLog are mutually exclusive - logfile will be used if present
	LogFile    string `envconfig:"log" json:"log"`
	DiscardLog bool   `envconfig:"nolog" json:"nolog"`

	// not yet implemented
	Background bool `envconfig:"background" json:"background"`
}

// IsMissingConfig returns true if the the error has to do with missing required configs
func IsMissingConfig(err error) bool {
	return strings.Contains(err.Error(), "required key")
}

// only for config from file
func setDefaults(c *Config) error {
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
					return fmt.Errorf("unable to convert default value to int: %v - err: %s", defaultValue, err)
				}
				if field.Int() == 0 && field.CanSet() {
					field.SetInt(int64(intVal))
				}
			case reflect.Bool:
				if field.CanSet() {
					v, err := strconv.ParseBool(defaultValue)
					if err != nil {
						return fmt.Errorf("unable to parse bool value for: %v - err: %s"+defaultValue, err)
					}
					field.SetBool(v)
				}
			default:
				log.Println("Unexpected type found in config.  Skipping: ", field)
			}
		}
	}

	return nil
}

// only for config from file
func validate(c *Config) error {
	val := reflect.ValueOf(c)
	refType := reflect.TypeOf(c)
	for i := 0; i < val.Elem().NumField(); i++ {
		field := val.Elem().Field(i)

		// required fields are all strings
		if _, ok := refType.Elem().Field(i).Tag.Lookup("required"); ok && field.String() == "" {
			return fmt.Errorf("required key %s missing value", refType.Elem().Field(i).Name)
		}
	}

	return nil
}

func configFromEnv() error {
	cfg := Config{}
	err := envconfig.Process("xmrwasp", &cfg)
	if err != nil {
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
	err = setDefaults(&cfg)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return errors.Wrap(err, "Failed to parse JSON.")
	}
	err = validate(&cfg)
	if err != nil {
		return err
	}

	instance = &cfg
	return nil
}

// Get returns the global configuration singleton.
func Get() *Config {
	var err error
	instantiation.Do(func() {
		if File != "" {
			f, err := os.Open(File)
			if err != nil {
				log.Fatal("open config file failed: ", err)
				return
			}
			defer f.Close()
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
