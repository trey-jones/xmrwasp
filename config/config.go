package config

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/kelseyhightower/envconfig"
	"github.com/pkg/errors"

	"go.uber.org/zap"
)

var (
	// File specifies a file from which to read the config
	// If empty, config will be read from the environment
	File string

	instance      *config
	oneTimeConfig = sync.Once{}

	ErrMissingRequiredConfig = errors.New("Required configuration option is missing.")
)

type config struct {
	DisableWebsocket bool   `envconfig:"noweb" json:"noweb"`
	DisableStratum   bool   `envconfig:"nostratum" default:"true" json:"nostratum"`
	WebsocketPort    string `envconfig:"wsport" default:"8080" json:"wsport"`
	StratumPort      string `envconfig:"strport" default:"1111" json:"strport"`

	// TODO multiple pools for fallback
	PoolAddr     string `envconfig:"url" required:"true" json:"url"`
	PoolLogin    string `envconfig:"login" required:"true" json:"login"`
	PoolPassword string `envconfig:"password" required:"true" json:"password"`

	StatInterval int `envconfig:"stats" default:"60" json:"stats"`

	DonateLevel int `envconfig:"donate" default:"3" json:"donate"`

	// not yet implemented
	Background bool
	LogFile    string
}

// This only needs to be run if read from JSON
func validateAndSetDefaults(c *config) error {
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
					zap.S().Error("Unable to convert default value to int: ", defaultValue)
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
				zap.S().Error("Unexpected type found in config.  Skipping: ", field)
			}
		}
		if _, ok := refType.Elem().Field(i).Tag.Lookup("required"); ok && field.String() == "" {
			zap.S().Error("Missing required field in config: ", refType.Elem().Field(i).Name)
			return ErrMissingRequiredConfig
		}
	}
	// zap.S().Debug("Configs after validation: ", c)
	return nil
}

func configFromEnv() error {
	cfg := config{}
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

	cfg := config{}
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return errors.Wrap(err, "Failed to parse JSON.")
	}
	// zap.S().Debug("config values after umarshal: ", cfg)
	err = validateAndSetDefaults(&cfg)
	if err != nil {
		return err
	}

	instance = &cfg
	return nil
}

func Get() *config {
	var err error
	oneTimeConfig.Do(func() {
		if File != "" {
			var f io.Reader
			f, err = os.Open(File)
			if err != nil {
				return
			}
			err = configFromFile(f)
		} else {
			// try to read config from environment
			err = configFromEnv()
		}
	})
	if err != nil {
		zap.S().Fatal("Unable to load config: ", err)
	}
	return instance
}
