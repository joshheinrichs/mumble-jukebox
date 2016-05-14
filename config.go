package main

import (
	"io/ioutil"

	"github.com/layeh/gumble/gumble"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Mumble     *gumble.Config
	Filesystem filesystem
}

type filesystem struct {
	Directory string
}

// NewConfig returns a new config with default settings.
func NewConfig() *Config {
	return &Config{
		Mumble: gumble.NewConfig(),
		Filesystem: filesystem{
			Directory: "cache",
		},
	}
}

// ReadConfig returns a new config with the default settings, overridden by the
// settings in the config file. The config should be in yaml format.
func ReadConfig(filename string) (*Config, error) {
	blob, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	config := NewConfig()
	err = yaml.Unmarshal(blob, config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
