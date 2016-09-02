package main

import "gopkg.in/ini.v1"

type Config struct {
	Mumble mumbleConfig `ini:"mumble"`
	Cache  cacheConfig  `ini:"cache"`
	Queue  queueConfig  `ini:"queue"`
}

type mumbleConfig struct {
	Username string `ini:"username"`
	Password string `ini:"password"`
	Address  string `ini:"address"`
	Port     string `ini:"port"`
}

type cacheConfig struct {
	Directory   string `ini:"directory"`
	MaxSize     int    `ini:"maxsize"`
	MaxFilesize string `ini:"maxfilesize"`
}

type queueConfig struct {
	MaxSize int `ini:"maxsize"`
}

// NewConfig returns a new config with default settings.
func NewConfig() *Config {
	return &Config{
		Mumble: mumbleConfig{
			Username: "Jukebox",
			Port:     "64738",
		},
		Cache: cacheConfig{
			Directory:   "~/.cache/mumble-jukebox",
			MaxFilesize: "100m",
			MaxSize:     10,
		},
		Queue: queueConfig{
			MaxSize: 50,
		},
	}
}

// ReadConfig returns a new config with the default settings, overridden by the
// settings in the config file..
func ReadConfig(filename string) (*Config, error) {
	cfg, err := ini.Load(filename)
	if err != nil {
		return nil, err
	}
	config := NewConfig()
	err = cfg.MapTo(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
