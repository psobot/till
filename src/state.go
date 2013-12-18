package main

import (
	"errors"
	"github.com/nu7hatch/gouuid"
	"log"
	"os"
	"sync"
	"time"
)

/*
 *  State
 */

type State struct {
	Config     *Config             `json:"config"`
	Providers  map[string]Provider `json:"providers"`
	Servers    map[string]Server   `json:"servers"`
	Identifier string              `json:"identifier"`
	Server     Server              `json:"server"`

	metadataMutex sync.RWMutex `json:"-"`
}

func NewState() State {
	u, _ := uuid.NewV4()
	return InitStateConfig(State{
		Servers:    make(map[string]Server),
		Identifier: u.String(),
	})
}

func InitStateConfig(state State) State {
	var err error

	provided_config := os.Getenv("TILL_CONFIG")
	if provided_config == "" {
		config_file := os.Getenv("TILL_CONFIG_FILE")
		if config_file == "" {
			state.Config, err = NewConfigFromJSONFile("./config.json")
		} else {
			state.Config, err = NewConfigFromJSONFile(config_file)
		}
	} else {
		state.Config, err = NewConfigFromJSON([]byte(provided_config))
	}

	if err != nil {
		log.Printf("Could not read config from JSON: %v", err)
	}

	providers := make(map[string]Provider)
	for _, pc := range state.Config.Providers {
		p, err := pc.NewProvider()
		if err == nil && p != nil {
			if _, exists := providers[pc.Name()]; exists {
				log.Printf("WARNING: Multiple providers exist with the same name (\"%v\"). Latter providers will not be used.", pc.Name())
			} else {
				log.Printf("Setting up %v provider \"%v\"...", pc.Type(), pc.Name())
				err = p.Connect()
				if err != nil {
					log.Printf("Could not connect %v provider \"%v\": %v", pc.Type(), pc.Name(), err)
				} else {
					providers[pc.Name()] = p
				}
			}
		} else {
			log.Printf("Could not instantiate %v provider \"%v\": %v", pc.Type(), pc.Name(), err)
		}
	}

	state.Providers = providers
	state.Server = NewServer(state.Identifier, state.Config.PublicAddress, 60)
	return state
}

func (s *State) AddServer(server Server) error {
	if server.Identifier == "" {
		panic("Server identifier cannot be empty!")
	} else if server.Identifier == state.Identifier {
		log.Printf("Not adding self to server list.")
		return errors.New("Not adding self to server list.")
	}

	exists := false

	s.metadataMutex.Lock()
	if _, exists = s.Servers[server.Identifier]; !exists {
		s.Servers[server.Identifier] = server
	}
	count := len(s.Servers)
	s.metadataMutex.Unlock()

	if !exists {
		log.Printf("Added server '%v'. Now at %d known till servers.", server.Identifier, count)
		return nil
	} else {
		log.Printf("Not re-adding server '%v'. Still at %d known till servers.", server.Identifier, count)
		return errors.New("Not re-adding to server list.")
	}
}

func (s *State) RemoveServerByID(id string) {
	s.metadataMutex.Lock()
	defer s.metadataMutex.Unlock()

	delete(s.Servers, id)
}

func (s *State) RemoveServerByAddr(addr string) {
	s.metadataMutex.Lock()
	defer s.metadataMutex.Unlock()

	for _, server := range s.Servers {
		if server.Address == addr {
			delete(s.Servers, server.Identifier)
			return
		}
	}
}

/*
 *	Server
 */

type Server struct {
	Identifier string
	Address    string
	Lifespan   int64

	added_at time.Time
}

func NewServer(id string, addr string, lifespan int64) Server {
	return Server{
		Identifier: id,
		Address:    addr,
		Lifespan:   lifespan,
		added_at:   time.Now(),
	}
}

func (s *Server) IsExpired() bool {
	return int64(time.Now().Sub(s.added_at)/time.Second) > s.Lifespan
}
