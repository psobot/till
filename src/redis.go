package main

import (
	"errors"
)

type RedisProviderConfig struct {
	BaseProviderConfig

	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database int    `json:"db"`

	MaxSize  int64 `json:"maxsize"`
	MaxItems int64 `json:"maxitems"`
}

func NewRedisProviderConfig(base BaseProviderConfig, data map[string]interface{}) (*RedisProviderConfig, error) {
	config := RedisProviderConfig{}

	config.BaseProviderConfig = base

	host, ok := data["host"]
	if ok {
		config.Host, ok = host.(string)
		if !ok {
			return nil, errors.New("Redis host must be a string.")
		}
	} else {
		config.Host = "localhost"
	}

	port, ok := data["port"]
	if ok {
		port, ok = port.(float64)
		if !ok {
			return nil, errors.New("Redis port must be a number.")
		} else {
			config.Port = int(port.(float64))
		}
	} else {
		config.Port = 6379
	}

	db, ok := data["db"]
	if ok {
		db, ok = db.(float64)
		if !ok {
			return nil, errors.New("Redis db must be a number.")
		} else {
			config.Database = int(db.(float64))
		}
	} else {
		config.Database = 0
	}

	maxsize, ok := data["maxsize"]
	if ok {
		maxsize, ok = maxsize.(float64)
		if !ok {
			return nil, errors.New("Redis maxsize must be a number.")
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
			return nil, errors.New("Redis maxitems must be a number.")
		} else {
			config.MaxItems = int64(maxitems.(float64))
		}
	} else {
		config.MaxItems = 0
	}

	return &config, nil
}
