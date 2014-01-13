package main

import (
	"bytes"
	"errors"
	"github.com/ncw/swift"
	"io"
	"strconv"
	"time"
)

type RackspaceProviderConfig struct {
	BaseProviderConfig

	RackspaceUserName  string `json:"rackspace_user_name"`
	RackspaceAPIKey    string `json:"rackspace_api_key"`
	RackspaceContainer string `json:"rackspace_container"`
	RackspaceRegion    string `json:"rackspace_region"`
	RackspacePrefix    string `json:"rackspace_prefix"`
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

	container, ok := data["rackspace_container"]
	if ok {
		config.RackspaceContainer, ok = container.(string)
		if !ok {
			return nil, errors.New("rackspace_container must be a string.")
		}
	} else {
		return nil, errors.New("rackspace_container must be defined.")
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

	prefix, ok := data["rackspace_prefix"]
	if ok {
		config.RackspacePrefix, ok = prefix.(string)
		if !ok {
			return nil, errors.New("rackspace_prefix must be a string.")
		}
	} else {
		config.RackspacePrefix = ""
	}

	return &config, nil
}

type RackspaceProvider struct {
	BaseProvider
	conn      swift.Connection
	container swift.Container
}

func (p *RackspaceProvider) GetConfig() RackspaceProviderConfig {
	return p.config.(RackspaceProviderConfig)
}

func (c RackspaceProviderConfig) NewProvider() (Provider, error) {
	r := &RackspaceProvider{
		BaseProvider: BaseProvider{c},
	}

	r.conn = swift.Connection{
		UserName: c.RackspaceUserName,
		AuthUrl:  "https://identity.api.rackspacecloud.com/v2.0",
		ApiKey:   c.RackspaceAPIKey,
	}
	err := r.conn.Authenticate()
	if err != nil {
		return nil, err
	} else {
		r.container, _, err = r.conn.Container(c.RackspaceContainer)
	}
	return r, nil
}

type RackspaceObject struct {
	BaseObject

	size   int64
	reader io.ReadCloser
}

func (s *RackspaceObject) GetSize() (int64, error) {
	return s.size, nil
}

func (s *RackspaceObject) Read(buf []byte) (int, error) {
	return s.reader.Read(buf)
}

func (s *RackspaceObject) Close() error {
	return s.reader.Close()
}

func (p *RackspaceProvider) Get(id string) (Object, error) {
	path := id

	var buf bytes.Buffer
	headers, err := p.conn.ObjectGet(p.container.Name, p.GetConfig().RackspacePrefix+path, &buf, true, nil)
	rc := NewDummyReadCloser(&buf)

	if err == swift.ObjectNotFound {
		return nil, nil
	} else if err != nil {
		return nil, err
	} else {
		md, _ := headers["X-Object-Meta-Till"]

		return &RackspaceObject{
			BaseObject: BaseObject{
				Metadata:   md,
				identifier: id,
				exists:     true,
				provider:   p,
			},
			reader: &rc,
			size:   int64(buf.Len()),
		}, nil
	}
}

func (p *RackspaceProvider) GetURL(id string) (Object, error) {
	//	TODO: Add GetURL support.
	return nil, nil
}

func (p *RackspaceProvider) Put(o Object) (Object, error) {
	//	TODO: Add path support within the container?

	now := time.Now().Unix()
	bo := o.GetBaseObject()
	expires := bo.Expires - now

	path := o.GetBaseObject().identifier
	size, err := o.GetSize()

	if err != nil {
		return nil, err
	} else {
		_, err := p.conn.ObjectPut(
			p.container.Name,
			p.GetConfig().RackspacePrefix+path,
			o,
			false,
			"",
			"application/octet-stream",
			swift.Headers{
				"Content-Length":     strconv.FormatInt(size, 10),
				"X-Delete-After":     strconv.FormatInt(expires, 10),
				"X-Object-Meta-Till": bo.Metadata,
			})
		return nil, err
	}
}

func (p *RackspaceProvider) Update(o Object) (Object, error) {
	return nil, nil
}
