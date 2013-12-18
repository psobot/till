package main

import (
	"github.com/garyburd/redigo/redis"
	"log"
	"os"
	"sync"
	//"time"
)

/*
 *  State
 */

type State struct {
	Config    *Config    `json:"config"`
	Providers []Provider `json:"providers"`

	//  Redis pool stuff
	pool    redis.Pool           `json:"-"`
	connGet chan chan redis.Conn `json:"-"`

	listenersGet    chan chan int `json:"-"`
	listenersAdd    chan int      `json:"-"`
	listenersRemove chan int      `json:"-"`
	listenersListen chan chan int

	CurrentListeners  int `json:"-"`
	listenerDelegates []chan int

	metadataMutex sync.RWMutex `json:"-"`
}

func NewState() State {
	state = State{
		connGet:           make(chan chan redis.Conn),
		listenersGet:      make(chan chan int),
		listenersAdd:      make(chan int),
		listenersRemove:   make(chan int),
		listenersListen:   make(chan chan int),
		listenerDelegates: make([]chan int, 0),
	}
	var err error

	state.Config, err = NewConfigFromJSON("./config.json")
	if err != nil {
		log.Printf("Could not read config from JSON: %v", err)
	}

	for _, pc := range state.Config.Providers {
		p, err := pc.NewProvider()
		if err == nil && p != nil {
			log.Printf("Setting up %v provider \"%v\"", pc.Type(), pc.Name())
			err = p.Connect()
			if err != nil {
				log.Printf("Could not connect %v provider \"%v\": %v", pc.Type(), pc.Name(), err)
			} else {
				state.Providers = append(state.Providers, p)
			}
		} else {
			log.Printf("Could not instantiate %v provider \"%v\": %v", pc.Type(), pc.Name(), err)
		}
	}
	return state
}

func (s *State) DispenseConnections() {
	for callback := range s.connGet {
		callback <- s.pool.Get()
	}
}

func (s *State) TallyListeners() {
	for {
		select {
		case request := <-s.listenersGet:
			request <- s.CurrentListeners
		case inc := <-s.listenersAdd:
			s.CurrentListeners += inc
			for _, delegate := range s.listenerDelegates {
				if delegate != nil {
					delegate <- s.CurrentListeners
				}
			}
		case dec := <-s.listenersRemove:
			s.CurrentListeners -= dec
			for _, delegate := range s.listenerDelegates {
				if delegate != nil {
					delegate <- s.CurrentListeners
				}
			}
		case delegate := <-s.listenersListen:
			s.listenerDelegates = append(s.listenerDelegates, delegate)
		}
	}
}

func (s *State) GetRedisConn(onConnection chan redis.Conn) {
	s.connGet <- onConnection
}

func (s *State) GetListenerCount(onCount chan int) {
	s.listenersGet <- onCount
}

func (s *State) AddListener() {
	s.listenersAdd <- 1
}

func (s *State) RemoveListener() {
	s.listenersRemove <- 1
}

func (s *State) ListenForListenerChanges(onChange chan int) {
	s.listenersListen <- onChange
}

func (s *State) Shutdown() {
	log.Println("Shutting down.")

	/*
		onCount := make(chan int)
		s.GetListenerCount(onCount)
		count := <-onCount

		if count > 0 {
			onChange := make(chan int)
			s.ListenForListenerChanges(onChange)
			log.Printf("Waiting for %d listeners to finish...", count)
			for count = range onChange {
				if count == 0 {
					break
				} else {
					log.Printf("Waiting for %d listeners to finish...", count)
				}
			}
		}

		log.Printf("No listeners connected. Exiting.")
	*/
	os.Exit(0)
}
