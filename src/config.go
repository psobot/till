package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
)

type ProviderConfig interface {
	Name() string
	Type() string
	NewProvider() (Provider, error)
}

type BaseProviderConfig struct {
	kind string `json:"type"`
	name string `json:"name"`
}

func (c BaseProviderConfig) Name() string {
	return c.name
}

func (c BaseProviderConfig) Type() string {
	return c.kind
}

func (c BaseProviderConfig) NewProvider() (Provider, error) {
	return nil, nil
}

func NewProviderConfig(data map[string]interface{}) ProviderConfig {
	kind := data["type"]

	config := BaseProviderConfig{
		kind: data["type"].(string),
		name: data["name"].(string),
	}

	var output ProviderConfig
	var err error

	switch kind {
	case "redis":
		output, err = NewRedisProviderConfig(config, data)
	case "file":
		output, err = NewFileProviderConfig(config, data)
	case "till":
		output, err = NewTillProviderConfig(config, data)
	case "s3":
		output, err = NewS3ProviderConfig(config, data)
	case "rackspace":
		output, err = NewRackspaceProviderConfig(config, data)
	default:
		log.Printf("WARNING: Could not handle provider info of type %s.", kind)
	}

	if err != nil {
		log.Printf("Could not parse provider from data %v\n%v", data, err)
		return nil
	} else {
		return output
	}
}

type BaseConfig struct {
	Port int    `json:"port"`
	Bind string `json:"bind"`
}

type IncomingConfig struct {
	BaseConfig

	Providers []interface{} `json:"providers"`
}

func (c *IncomingConfig) toConfig() *Config {
	newProviders := make([]ProviderConfig, 0)

	for _, provider := range c.Providers {
		if p, ok := provider.(map[string]interface{}); ok {
			if pc := NewProviderConfig(p); pc != nil {
				newProviders = append(newProviders, pc)
			}
		} else {
			log.Printf("Invalid JSON data in provider configuration. Expected object, got %v", provider)
		}
	}

	config := &Config{}
	config.Port = c.Port
	config.Bind = c.Bind
	config.Providers = newProviders
	return config
}

type Config struct {
	BaseConfig

	Providers []ProviderConfig `json:"providers"`
}

func NewConfigFromJSON(config string) (*Config, error) {
	file, e := ioutil.ReadFile("./config.json")
	if e != nil {
		return nil, e
	}

	tmp_config := &IncomingConfig{}

	e = json.Unmarshal(file, tmp_config)
	if e != nil {
		return nil, e
	} else {
		return tmp_config.toConfig(), nil
	}
}
