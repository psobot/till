package main

import (
	"errors"
	"github.com/garyburd/redigo/redis"
	"io"
	"log"
	"strconv"
	"strings"
	"time"
)

type RedisProviderConfig struct {
	BaseProviderConfig

	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database int    `json:"db"`
	Password string `json:"password"`

	MaxItems int `json:"maxitems"`
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

	password, ok := data["password"]
	if ok {
		config.Password, ok = password.(string)
		if !ok {
			return nil, errors.New("Redis password must be a string.")
		}
	} else {
		config.Password = ""
	}

	maxitems, ok := data["maxitems"]
	if ok {
		maxitems, ok = maxitems.(float64)
		if !ok {
			return nil, errors.New("Redis maxitems must be a number.")
		} else {
			config.MaxItems = int(maxitems.(float64))
		}
	} else {
		config.MaxItems = 0
	}

	return &config, nil
}

type RedisProvider struct {
	BaseProvider

	pool                 redis.Pool
	countScript          *redis.Script
	randomValueKeyScript *redis.Script
}

func (c RedisProviderConfig) NewProvider() (Provider, error) {
	p := &RedisProvider{
		BaseProvider: BaseProvider{c},
	}

	p.pool = redis.Pool{
		MaxIdle:     10,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", c.Host+":"+strconv.Itoa(c.Port))
			if err != nil {
				return nil, err
			}
			if len(c.Password) > 0 {
				if _, err := conn.Do("AUTH", c.Password); err != nil {
					conn.Close()
					return nil, err
				}
			}

			if _, err := conn.Do("SELECT", c.Database); err != nil {
				conn.Close()
				return nil, err
			}
			return conn, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	p.countScript = redis.NewScript(0, `return #redis.call('keys', '::till:value:*')`)
	p.randomValueKeyScript = redis.NewScript(0, `
		local keys = redis.call('keys', '::till:value:*')
if #keys > 0 then
    return keys[1]
else
    return nil
end
	`)
	return p, nil
}

func (p *RedisProvider) GetConfig() RedisProviderConfig {
	return p.config.(RedisProviderConfig)
}

func (p *RedisProvider) KeyForObject(key string) string {
	return "::till:value:" + key
}

func (p *RedisProvider) KeyForMetadata(key string) string {
	return "::till:metadata:" + key
}

func (p *RedisProvider) GetObjectCount(c redis.Conn) (int, error) {
	return redis.Int(p.countScript.Do(c))
}

func (p *RedisProvider) Get(id string) (Object, error) {
	c := p.pool.Get()
	exists, err := redis.Bool(c.Do("EXISTS", p.KeyForObject(id)))
	if err != nil {
		return nil, err
	} else if exists {
		metadata, _ := redis.String(c.Do("GET", p.KeyForMetadata(id)))
		return &RedisObject{
			BaseObject: BaseObject{
				Metadata:   metadata,
				identifier: id,
				exists:     true,
				provider:   p,
			},
			c:           c,
			objectKey:   p.KeyForObject(id),
			metadataKey: p.KeyForMetadata(id),
		}, nil
	} else {
		return nil, nil
	}
}

func (p *RedisProvider) GetURL(id string) (Object, error) {
	return nil, nil
}

func (p *RedisProvider) RemoveOldest(c redis.Conn) {
	keyb, err := redis.String(p.randomValueKeyScript.Do(c))
	if err != nil {
		log.Printf("Error: %v", err)
	} else {
		key := string(keyb)
		id := strings.Replace(key, "::till:value:", "", 1)

		_, err := c.Do("DEL", p.KeyForMetadata(id), p.KeyForObject(id))
		if err != nil {
			log.Printf("Could not remove keys for object %v: %v", id, err)
		}
	}
}

func (p *RedisProvider) Put(o Object) (Object, error) {
	c := p.pool.Get()
	defer c.Close()

	maxItems := p.GetConfig().MaxItems
	if maxItems > 0 {
		for {
			count, err := p.GetObjectCount(c)
			if err != nil {
				return nil, err
			} else {
				if count+1 > maxItems {
					p.RemoveOldest(c)
				} else {
					break
				}
			}
		}
	}

	now := time.Now().Unix()
	bo := o.GetBaseObject()
	expires := bo.Expires - now

	exists, err := redis.Bool(c.Do("EXISTS", p.KeyForMetadata(bo.identifier)))
	if err != nil {
		return nil, err
	}

	if exists {
		return p.Update(o)
	} else {
		exists, err = redis.Bool(c.Do("EXISTS", p.KeyForObject(bo.identifier)))
		if err != nil {
			return nil, err
		}
		if exists {
			return p.Update(o)
		}
	}

	_, err = c.Do(
		"SETEX",
		p.KeyForMetadata(bo.identifier),
		expires,
		o.GetBaseObject().Metadata,
	)
	if err != nil {
		return nil, err
	} else {
		length, err := o.GetSize()
		if err != nil {
			return nil, err
		}

		//	TODO: not this
		data := make([]byte, length)

		_, err = o.Read(data)
		if err != nil && err != io.EOF {
			return nil, err
		}

		_, err = c.Do(
			"SETEX",
			p.KeyForObject(bo.identifier),
			expires,
			data,
		)

		if err != nil {
			return nil, err
		}

		return &RedisObject{
			BaseObject:  bo,
			c:           p.pool.Get(),
			objectKey:   p.KeyForObject(bo.identifier),
			metadataKey: p.KeyForMetadata(bo.identifier),
		}, nil
	}
}

func (p *RedisProvider) Update(o Object) (Object, error) {
	c := p.pool.Get()
	defer c.Close()

	now := time.Now().Unix()
	bo := o.GetBaseObject()
	expires := bo.Expires - now

	_, err := c.Do(
		"EXPIRE",
		p.KeyForMetadata(bo.identifier),
		expires,
	)
	if err != nil {
		return nil, err
	}

	_, err = c.Do(
		"EXPIRE",
		p.KeyForObject(bo.identifier),
		expires,
	)
	if err != nil {
		return nil, err
	}

	return o, nil
}

type RedisObject struct {
	BaseObject  `json:"base"`
	c           redis.Conn
	objectKey   string
	metadataKey string

	tell int
}

func (r *RedisObject) GetProvider() *RedisProvider {
	return r.BaseObject.provider.(*RedisProvider)
}

func (r *RedisObject) GetSize() (int64, error) {
	stat, err := redis.Int(r.c.Do("STRLEN", r.objectKey))
	if err != nil {
		return -1, err
	} else {
		return int64(stat), nil
	}
}

func (r *RedisObject) Read(b []byte) (int, error) {
	length := len(b)
	data, err := redis.Bytes(r.c.Do("GETRANGE", r.objectKey, r.tell, r.tell+length-1))
	r.tell += len(data)
	if err != nil {
		return -1, err
	} else {
		return copy(b, data), nil
	}
}

func (r *RedisObject) Close() error {
	return r.c.Close()
}
