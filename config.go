package main

import (
	"github.com/layeh/gumble/gumble"
)

type Config struct {
	Mumble     *gumble.Config
	Filesystem filesystem
}

type filesystem struct {
	Directory string
}

func NewConfig() *Config {
	return &Config{
		Mumble: gumble.NewConfig(),
		Filesystem: filesystem{
			Directory: "cache",
		},
	}
}
