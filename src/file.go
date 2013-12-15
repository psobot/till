package main

import (
	"errors"
)

type FileProviderConfig struct {
	BaseProviderConfig

	Path string `json:"path"`

	MaxSize  int64 `json:"maxsize"`
	MaxItems int64 `json:"maxitems"`
}

func NewFileProviderConfig(base BaseProviderConfig, data map[string]interface{}) (*FileProviderConfig, error) {
	config := FileProviderConfig{}

	config.BaseProviderConfig = base

	path, ok := data["path"]
	if ok {
		config.Path, ok = path.(string)
		if !ok {
			return nil, errors.New("File path must be a string.")
		}
	} else {
		config.Path = "/var/cache/till"
	}

	maxsize, ok := data["maxsize"]
	if ok {
		maxsize, ok = maxsize.(float64)
		if !ok {
			return nil, errors.New("File maxsize must be a number.")
		} else {
			config.MaxSize = int64(maxsize.(float64))
		}
	} else {
		config.MaxSize = 0
	}

	maxitems, ok := data["maxitems"]
	if ok {
		maxitems, ok = maxitems.(float64)
		if !ok {
			return nil, errors.New("File maxitems must be a number.")
		} else {
			config.MaxItems = int64(maxitems.(float64))
		}
	} else {
		config.MaxItems = 0
	}

	return &config, nil
}
