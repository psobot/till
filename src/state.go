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
	Config Config `json:"config"`

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
	state.Config.ReadFromJSON("./config.json")

	/*
		state.pool = redis.Pool{
			MaxIdle:     10,
			IdleTimeout: 240 * time.Second,
			Dial: func() (redis.Conn, error) {
				c, err := redis.Dial("tcp", state.Config.RedisHostPort)
				if err != nil {
					return nil, err
				}
				if _, err := c.Do("AUTH", state.Config.RedisPassword); err != nil {
					c.Close()
					return nil, err
				}
				return c, err
			},
			TestOnBorrow: func(c redis.Conn, t time.Time) error {
				_, err := c.Do("PING")
				return err
			},
		}
		go state.DispenseConnections()
		go state.TallyListeners()
	*/
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
	os.Exit(0)
}
