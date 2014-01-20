package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"regexp"
)

type ProviderConfig interface {
	Name() string
	Type() string
	NewProvider() (Provider, error)
	AcceptsKey(key string) bool
}

type BaseProviderConfig struct {
	kind      string           `json:"type"`
	name      string           `json:"name"`
	whitelist []*regexp.Regexp `json:"whitelist"`
}

func (c BaseProviderConfig) Name() string {
	return c.name
}

func (c BaseProviderConfig) Type() string {
	return c.kind
}

func (c BaseProviderConfig) AcceptsKey(key string) bool {
	for _, pattern := range c.whitelist {
		if pattern.MatchString(key) {
			return true
		}
	}
	return false
}

func (c BaseProviderConfig) NewProvider() (Provider, error) {
	return nil, nil
}

func NewProviderConfig(data map[string]interface{}) ProviderConfig {
	kind := data["type"]

	whitelist := make([]*regexp.Regexp, 0)
	if src, exists := data["whitelist"]; exists {
		if patterns, ok := src.([]interface{}); ok {
			for _, obj := range patterns {
				if pattern, ok := obj.(string); ok {
					r, err := regexp.Compile(pattern)
					if err != nil {
						log.Printf("Could not compile regex \"%v\": %v", pattern, err)
					} else {
						whitelist = append(whitelist, r)
					}
				} else {
					log.Printf("Non-string whitelist entry not found: %v", obj)
				}
			}
		} else {
			log.Printf("Whitelist for provider %v is not a list.", data["name"])
		}
	}

	config := BaseProviderConfig{
		kind:      data["type"].(string),
		name:      data["name"].(string),
		whitelist: whitelist,
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
	Port                      int    `json:"port"`
	Bind                      string `json:"bind"`
	DefaultLifespan           int    `json:"default_lifespan"`
	PublicAddress             string `json:"public_address"`
	GetTimeoutInMilliseconds  int    `json:"get_timeout_in_milliseconds"`
	PostTimeoutInMilliseconds int    `json:"post_timeout_in_milliseconds"`
}

type IncomingConfig struct {
	BaseConfig

	Providers        []interface{}      `json:"providers"`
	LifespanPatterns map[string]float64 `json:"lifespan_patterns"`
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

	lifespanPatterns := make(map[*regexp.Regexp]float64, len(c.LifespanPatterns))
	for pattern, lifespan := range c.LifespanPatterns {
		p, err := regexp.Compile(pattern)
		if err != nil {
			log.Printf("Invalid regexp \"%v\" in provider configuration: %v", pattern, err)
		} else {
			lifespanPatterns[p] = lifespan
		}
	}

	config := &Config{}
	//	TODO: Not this
	config.Port = c.Port
	config.Bind = c.Bind
	config.Providers = newProviders
	config.DefaultLifespan = c.DefaultLifespan
	config.LifespanPatterns = lifespanPatterns
	config.PublicAddress = c.PublicAddress

	if c.GetTimeoutInMilliseconds > 0 {
		config.GetTimeoutInMilliseconds = c.GetTimeoutInMilliseconds
	} else {
		config.GetTimeoutInMilliseconds = 1000
	}
	if c.PostTimeoutInMilliseconds > 0 {
		config.PostTimeoutInMilliseconds = c.PostTimeoutInMilliseconds
	} else {
		config.PostTimeoutInMilliseconds = 1000
	}

	return config
}

type Config struct {
	BaseConfig

	Providers        []ProviderConfig           `json:"providers"`
	LifespanPatterns map[*regexp.Regexp]float64 `json:"lifespan_patterns"`
}

func NewConfigFromJSONFile(configfile string) (*Config, error) {
	file, e := ioutil.ReadFile(configfile)
	if e != nil {
		return nil, e
	}
	return NewConfigFromJSON(file)
}

func NewConfigFromJSON(config []byte) (*Config, error) {
	tmp_config := &IncomingConfig{}

	e := json.Unmarshal(config, tmp_config)
	if e != nil {
		return nil, e
	} else {
		return tmp_config.toConfig(), nil
	}
}
