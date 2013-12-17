package main

import (
	"errors"
)

type RackspaceProviderConfig struct {
	BaseProviderConfig

	RackspaceUserName string `json:"rackspace_user_name"`
	RackspaceAPIKey   string `json:"rackspace_api_key"`
	RackspaceRegion   string `json:"rackspace_region"`
}

func NewRackspaceProviderConfig(base BaseProviderConfig, data map[string]interface{}) (*RackspaceProviderConfig, error) {
	config := RackspaceProviderConfig{}

	config.BaseProviderConfig = base

	username, ok := data["rackspace_user_name"]
	if ok {
		config.RackspaceUserName, ok = username.(string)
		if !ok {
			return nil, errors.New("rackspace_user_name must be a string.")
		}
	} else {
		return nil, errors.New("rackspace_user_name must be defined.")
	}

	api_key, ok := data["rackspace_api_key"]
	if ok {
		config.RackspaceAPIKey, ok = api_key.(string)
		if !ok {
			return nil, errors.New("rackspace_api_key must be a string.")
		}
	} else {
		return nil, errors.New("rackspace_api_key must be defined.")
	}

	region, ok := data["rackspace_region"]
	if ok {
		config.RackspaceRegion, ok = region.(string)
		if !ok {
			return nil, errors.New("rackspace_region must be a string.")
		}
	} else {
		config.RackspaceRegion = "ORD"
	}

	return &config, nil
}

type RackspaceProvider struct {
	BaseProvider
}

func (c *RackspaceProviderConfig) NewProvider() (Provider, error) {
	return &RackspaceProvider{BaseProvider{c}}, nil
}

func (p *RackspaceProvider) Get(id string) (Object, error) {
	return nil, nil
}

func (p *RackspaceProvider) GetURL(id string) (Object, error) {
	return nil, nil
}

func (p *RackspaceProvider) Put(o Object) (Object, error) {
	return nil, nil
}

func (p *RackspaceProvider) Update(o Object) (Object, error) {
	return nil, nil
}
