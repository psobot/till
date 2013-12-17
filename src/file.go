package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"time"
)

type FileProviderConfig struct {
	BaseProviderConfig

	Path string `json:"path"`

	MaxSize  int64 `json:"maxsize"`
	MaxItems int64 `json:"maxitems"`
}

func NewFileProviderConfig(base BaseProviderConfig, data map[string]interface{}) (*FileProviderConfig, error) {
	config := FileProviderConfig{}

	config.BaseProviderConfig = base

	path, ok := data["path"]
	if ok {
		config.Path, ok = path.(string)
		if !ok {
			return nil, errors.New("File path must be a string.")
		}
	} else {
		config.Path = "/var/cache/till"
	}

	maxsize, ok := data["maxsize"]
	if ok {
		maxsize, ok = maxsize.(float64)
		if !ok {
			return nil, errors.New("File maxsize must be a number.")
		} else {
			config.MaxSize = int64(maxsize.(float64))
		}
	} else {
		config.MaxSize = 0
	}

	maxitems, ok := data["maxitems"]
	if ok {
		maxitems, ok = maxitems.(float64)
		if !ok {
			return nil, errors.New("File maxitems must be a number.")
		} else {
			config.MaxItems = int64(maxitems.(float64))
		}
	} else {
		config.MaxItems = 0
	}

	return &config, nil
}

type FileProvider struct {
	BaseProvider

	//  Internal stuff that should only be touched from the goroutine
	cache           map[string]int64
	sortedCacheKeys []string
	nextTimestamp   int64
	currentSize     int64

	add    chan *FileObject
	update chan *FileObject
	done   chan bool
}

func (c FileProviderConfig) NewProvider() (Provider, error) {
	return &FileProvider{
		BaseProvider: BaseProvider{c},

		cache:           make(map[string]int64),
		sortedCacheKeys: make([]string, 0),
		nextTimestamp:   -1,
		currentSize:     0,

		add:    make(chan *FileObject, 10),
		update: make(chan *FileObject, 10),
		done:   make(chan bool),
	}, nil
}

func (p *FileProvider) GetConfig() FileProviderConfig {
	return p.config.(FileProviderConfig)
}

func (p *FileProvider) Connect() error {
	e := os.MkdirAll(p.GetMetadataPath(""), os.ModeDir|os.ModePerm)
	if e != nil {
		return e
	}

	e = os.MkdirAll(p.GetMetadataPath(""), os.ModeDir|os.ModePerm)
	if e != nil {
		return e
	}
	go p.StartExpiryLoop()
	return nil
}

func (p *FileProvider) StartExpiryLoop() {
	d, err := os.Open(p.GetFilePath(""))
	log.Printf("Dir %v", d)
	if err != nil {
		log.Fatalf("Error in expiry loop: %v", err)
		return
	}

	fi, err := d.Readdir(-1)
	if err != nil {
		log.Fatalf("Error in expiry loop: %v", err)
		return
	}

	for _, fi := range fi {
		log.Printf("File %v", fi)
		if fi.Mode().IsRegular() {
			o, err := p.LoadMetadata(fi.Name())
			if err != nil {
				log.Printf("Could not load metadata for object: %v", fi.Name())
			} else {
				p.cache[fi.Name()] = o.Expires
				stat, err := os.Stat(p.GetFilePath(fi.Name()))
				if err != nil {
					log.Printf("Could not read size of object %v.", fi.Name())
				} else {
					p.currentSize += stat.Size()
				}
			}
		}
	}

	p.sortedCacheKeys = p.GetSortedCacheKeys()
	p.Expire()

	for {
		sleepFor := p.NextSleepDuration()
		select {
		case ob := <-p.add:
			p.cache[ob.identifier] = ob.Expires
			p.sortedCacheKeys = p.GetSortedCacheKeys()
			p.nextTimestamp = p.GetNextTimestamp()

			stat, err := os.Stat(p.GetFilePath(ob.identifier))
			if err != nil {
				log.Printf("Could not read size of object %v.", ob.identifier)
			} else {
				p.currentSize += stat.Size()
			}

			maxItems := p.GetConfig().MaxItems
			if maxItems > 0 {
				for {
					if int64(len(p.cache)) < maxItems {
						break
					} else {
						p.RemoveOldest()
					}
				}
			}

			maxSize := p.GetConfig().MaxSize
			if maxSize > 0 {
				for {
					if p.currentSize < maxSize {
						break
					} else {
						p.RemoveOldest()
					}
				}
			}
		case ob := <-p.update:
			p.sortedCacheKeys = p.GetSortedCacheKeys()
			p.nextTimestamp = p.GetNextTimestamp()
			p.cache[ob.identifier] = ob.Expires
		case <-p.done:
			break
		case <-time.After(sleepFor):
			p.sortedCacheKeys = p.GetSortedCacheKeys()
			p.nextTimestamp = p.GetNextTimestamp()
			p.Expire()
			break
		}
	}
}

func (p *FileProvider) NextSleepDuration() time.Duration {
	now := time.Now().Unix()
	if p.nextTimestamp > 0 {
		secs := p.nextTimestamp - now
		if secs > 0 {
			return (time.Duration(secs) * time.Second)
		} else {
			log.Printf("Warning: timeout set in past: %d", secs)
			return 60 * time.Second
		}
	} else {
		return 60 * 60 * time.Second
	}
}

func (p *FileProvider) RemoveOldest() {
	if len(p.sortedCacheKeys) > 0 {
		lastIndex := len(p.sortedCacheKeys) - 1
		if lastIndex < 0 {
			return
		}
		p.Remove(p.sortedCacheKeys[lastIndex])
		maxElement := len(p.sortedCacheKeys) - 2
		if maxElement < 0 {
			maxElement = 0
		}
		p.sortedCacheKeys = p.sortedCacheKeys[0:maxElement]
	}
}

func (p *FileProvider) Remove(key string) error {
	stat, err := os.Stat(p.GetFilePath(key))
	if err != nil {
		log.Printf("Could not read size of object %v.", key)
		return err
	} else {
		p.currentSize -= stat.Size()
	}

	err = os.Remove(p.GetFilePath(key))
	if err != nil {
		log.Printf("Could not remove object %v: %v", key, err)
		return err
	} else {
		log.Printf("Removed object %v.", key)
		err = os.Remove(p.GetMetadataPath(key))
		if err != nil {
			log.Printf("Could not remove metadata for %v: %v", key, err)
			return err
		} else {
			delete(p.cache, key)
			return nil
		}
	}
}

type SortedMap struct {
	m map[string]int64
	s []string
}

func (sm *SortedMap) Len() int {
	return len(sm.m)
}

func (sm *SortedMap) Less(i, j int) bool {
	return sm.m[sm.s[i]] > sm.m[sm.s[j]]
}

func (sm *SortedMap) Swap(i, j int) {
	sm.s[i], sm.s[j] = sm.s[j], sm.s[i]
}

func (p *FileProvider) GetNextTimestamp() int64 {
	keys := p.sortedCacheKeys
	if len(keys) > 0 {
		return p.cache[keys[0]]
	} else {
		return -1
	}
}

func (p *FileProvider) GetSortedCacheKeys() []string {
	sm := new(SortedMap)
	sm.m = p.cache
	sm.s = make([]string, len(sm.m))
	i := 0
	for key, _ := range sm.m {
		sm.s[i] = key
		i++
	}
	sort.Sort(sort.Reverse(sm))
	return sm.s
}

func (p *FileProvider) Expire() {
	now := time.Now().Unix()
	for _, key := range p.sortedCacheKeys {
		expiry := p.cache[key]
		if expiry <= now {
			p.nextTimestamp = -1
			err := p.Remove(key)
			if err != nil {
				log.Printf("Could not expire key %v: %v", key, err)
				continue
			}
		} else {
			p.nextTimestamp = expiry
			break
		}
	}
}

func (p *FileProvider) StopExpiryLoop() {
	p.done <- true
}

func (p *FileProvider) GetFilePath(id string) string {
	return p.GetConfig().Path + "/files/" + id
}

func (p *FileProvider) GetMetadataPath(id string) string {
	return p.GetConfig().Path + "/metadata/" + id + ".json"
}

func (p *FileProvider) SaveMetadata(o FileObject) error {
	data, err := json.Marshal(o)
	if err != nil {
		return err
	} else {
		metadataPath := p.GetMetadataPath(o.BaseObject.identifier)
		f, err := os.Create(metadataPath)
		if err != nil {
			return err
		} else {
			l, err := f.Write(data)
			if err != nil || l < len(data) {
				os.Remove(metadataPath)
				return err
			} else {
				return nil
			}
		}
	}
}

func (p *FileProvider) LoadMetadata(id string) (*FileObject, error) {
	file, e := ioutil.ReadFile(p.GetMetadataPath(id))
	if e != nil {
		return nil, e
	}

	fo := &FileObject{}

	e = json.Unmarshal(file, fo)
	if e != nil {
		return nil, e
	} else {
		fo.BaseObject.identifier = id
		fo.BaseObject.exists = true
		fo.BaseObject.provider = p
		return fo, nil
	}
}

func (p *FileProvider) Get(id string) (Object, error) {
	file, err := os.Open(p.GetFilePath(id))
	if err != nil {
		return nil, err
	} else {
		obj, err := p.LoadMetadata(id)
		if err != nil {
			return nil, err
		} else {
			obj.File = file
			return obj, nil
		}
	}
}

func (p *FileProvider) GetURL(id string) (Object, error) {
	return nil, nil
}

func (p *FileProvider) Put(o Object) (Object, error) {
	file, err := os.Create(p.GetFilePath(o.GetBaseObject().identifier))
	if err != nil {
		return nil, err
	} else {
		fo := FileObject{
			BaseObject: o.GetBaseObject(),
			File:       file,
		}
		for {
			data, err := o.Read(4096)
			if len(data) == 0 || err != nil {
				break
			} else {
				file.Write(data)
			}
		}

		if err != nil {
			os.Remove(file.Name())
			return nil, err
		}

		err = file.Close()
		if err != nil {
			os.Remove(file.Name())
			return nil, err
		} else {
			err = p.SaveMetadata(fo)
			if err != nil {
				return nil, err
			} else {
				p.add <- &fo
				return &fo, nil
			}
		}
	}
}

func (p *FileProvider) Update(o Object) (Object, error) {
	return nil, nil
}

type FileObject struct {
	BaseObject `json:"base"`

	File *os.File `json:"-"`
}

func (f *FileObject) GetSize() (int64, error) {
	stat, err := f.File.Stat()
	if err != nil {
		return -1, err
	} else {
		return stat.Size(), nil
	}
}

func (f *FileObject) Read(length int) ([]byte, error) {
	buf := make([]byte, length)
	len, err := f.File.Read(buf)
	if err != nil {
		return []byte{}, err
	} else {
		return buf[:len], nil
	}
}
