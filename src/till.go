package main

import (
	"errors"
)

type TillProviderConfig struct {
	BaseProviderConfig

	RequestTypes []string `json:"request_types"`
	Servers      []string `json:"servers"`
}

func NewTillProviderConfig(base BaseProviderConfig, data map[string]interface{}) (*TillProviderConfig, error) {
	config := TillProviderConfig{}

	config.BaseProviderConfig = base

	types, ok := data["request_types"]
	if ok {
		if interfaces, ok := types.([]interface{}); ok {
			config.RequestTypes = make([]string, 0)

			for _, i := range interfaces {
				if s, ok := i.(string); ok {
					config.RequestTypes = append(config.RequestTypes, s)
				} else {
					return nil, errors.New("Request types must be a list of strings.")
				}
			}
		} else {
			return nil, errors.New("Request types must be a list of strings.")
		}
	} else {
		//  By default, only check the "file" and "redis" providers.
		config.RequestTypes = []string{
			"file", "redis",
		}
	}

	servers, ok := data["servers"]
	if ok {
		if interfaces, ok := servers.([]interface{}); ok {
			config.Servers = make([]string, 0)
			for _, i := range interfaces {
				if s, ok := i.(string); ok {
					config.Servers = append(config.Servers, s)
				} else {
					return nil, errors.New("Server list must be a list of strings.")
				}
			}
		} else {
			return nil, errors.New("Server list must be a list of strings.")
		}
	} else {
		config.Servers = []string{}
	}

	return &config, nil
}

type TillProvider struct {
	BaseProvider
}

func (c *TillProviderConfig) NewProvider() (Provider, error) {
	return &TillProvider{BaseProvider{c}}, nil
}

func (p *TillProvider) Get(id string) (Object, error) {
	return nil, nil
}

func (p *TillProvider) GetURL(id string) (Object, error) {
	return nil, nil
}

func (p *TillProvider) Put(o Object) (Object, error) {
	return nil, nil
}

func (p *TillProvider) Update(o Object) (Object, error) {
	return nil, nil
}
