package main

import (
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
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
		config.RequestTypes = []string{}
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
	return &TillProvider{
		BaseProvider: BaseProvider{c},
	}, nil
}

func (p *TillProvider) GetConfig() *TillProviderConfig {
	return p.config.(*TillProviderConfig)
}

func (p *TillProvider) OnServerUp() {
	go func() {
		for _, server := range p.GetConfig().Servers {
			go func(addr string) {
				NotifyServer(state.Server, NewServer("", addr, 60))
			}(server)
		}
	}()
}

func (p *TillProvider) Get(id string) (Object, error) {
	//	Query the other known Till servers and ask for requests by name.
	//	If any return errors or are not connectable, remove them from the list.
	//	Return objects from the first server to respond with an object.
	results := make(chan Object, 0)

	servers := make([]Server, 0)
	for _, server := range state.Servers {
		servers = append(servers, server)
	}

	if len(servers) == 0 {
		for _, server_addr := range p.GetConfig().Servers {
			servers = append(servers, NewServer("", server_addr, 60))
		}
	}

	if len(servers) > 0 {
		for _, server := range servers {
			go p.queryServer(id, server, results)
		}

		//	TODO: Make me configurable
		timeout := 2000
		endtime := time.Now().Add(time.Duration(timeout) * time.Millisecond)

		for {
			select {
			case r := <-results:
				if r != nil {
					close(results)
					return r, nil
				}
			case <-time.After(endtime.Sub(time.Now())):
				close(results)
				return nil, nil
			}
		}
	}
	return nil, nil
}

type TillObject struct {
	BaseObject

	size   int64
	reader io.ReadCloser
}

func (s *TillObject) GetSize() (int64, error) {
	return s.size, nil
}

func (s *TillObject) Read(buf []byte) (int, error) {
	return s.reader.Read(buf)
}

func (s *TillObject) Close() error {
	return s.reader.Close()
}

func (p *TillProvider) queryServer(id string, server Server, results chan Object) {
	//	Let's supress any panics in this function caused by
	//	putting objects into a closed channel.
	defer func() {
		recover()
	}()

	client := &http.Client{}
	req, err := http.NewRequest("GET", "http://"+server.Address+"/api/v1/object/"+id, nil)
	if err != nil {
		log.Printf("Error making new outgoing Till request: %v", err)
	} else {
		if len(p.GetConfig().RequestTypes) > 0 {
			req.Header.Add("X-Till-Providers", strings.Join(p.GetConfig().RequestTypes, ","))
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Error making new outgoing Till request: %v", err)
		} else {
			if resp.StatusCode == 200 {
				results <- &TillObject{
					BaseObject: BaseObject{
						Metadata:   resp.Header.Get("X-Till-Metadata"),
						identifier: id,
						exists:     true,
						provider:   p,
					},
					reader: resp.Body,
					size:   resp.ContentLength,
				}
			} else {
				results <- nil
				resp.Body.Close()
			}
		}
	}

	results <- nil
}

func (p *TillProvider) GetURL(id string) (Object, error) {
	//	TODO: This should return the same URL being queried.
	return nil, nil
}

func (p *TillProvider) Put(o Object) (Object, error) {
	return nil, nil
}

func (p *TillProvider) Update(o Object) (Object, error) {
	return nil, nil
}
