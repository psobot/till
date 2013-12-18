package main

import (
	"encoding/json"
	"errors"
	"io"
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
		log.Printf("Could not make dir '%v': %v", p.GetMetadataPath(""), e)
		return e
	}

	e = os.MkdirAll(p.GetFilePath(""), os.ModeDir|os.ModePerm)
	if e != nil {
		log.Printf("Could not make dir '%v': %v", p.GetFilePath(""), e)
		return e
	}
	go p.StartExpiryLoop()
	return nil
}

func (p *FileProvider) StartExpiryLoop() {
	d, err := os.Open(p.GetFilePath(""))
	if err != nil {
		log.Printf("Error in expiry loop: %v", err)
		return
	}

	fi, err := d.Readdir(-1)
	if err != nil {
		log.Printf("Error in expiry loop: %v", err)
		return
	}

	for _, fi := range fi {
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
						log.Printf("Removing oldest item - max size %d exceeded by %d.", maxSize, p.currentSize)
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
			return 0
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
		log.Printf("Removing object %v from local filesystem cache.", key)
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
	if len(id) > 0 {
		return p.GetConfig().Path + "/metadata/" + id + ".json"
	} else {
		return p.GetConfig().Path + "/metadata/"
	}
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
				f.Close()
				return err
			} else {
				f.Close()
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
	if err != nil || file == nil {
		if file != nil {
			file.Close()
		}
		if os.IsNotExist(err) {
			return nil, nil
		} else {
			return nil, err
		}
	} else {
		obj, err := p.LoadMetadata(id)
		if err != nil || file == nil {
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
	objectPath := p.GetFilePath(o.GetBaseObject().identifier)

	if _, exists := p.cache[o.GetBaseObject().identifier]; exists {
		return p.Update(o)
	} else {
		file, err := os.OpenFile(objectPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
		if file != nil {
			defer file.Close()
		}
		if err != nil {
			if os.IsExist(err) {
				return p.Update(o)
			} else {
				return nil, err
			}
		} else {
			fo := FileObject{
				BaseObject: o.GetBaseObject(),
				File:       file,
			}
			data := make([]byte, 4096)
			for {
				length, err := o.Read(data)
				if length == 0 || (err != nil && err != io.EOF) {
					break
				} else {
					file.Write(data[0:length])
				}

				if err == io.EOF {
					break
				}
			}

			if err != nil {
				os.Remove(file.Name())
				return nil, err
			}

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
	bo := o.GetBaseObject()

	fo := FileObject{
		BaseObject: bo,
	}

	if _, ok := p.cache[bo.identifier]; ok {
		p.add <- &fo
		err := p.SaveMetadata(fo)
		if err != nil {
			return nil, err
		} else {
			return &fo, nil
		}
	} else {
		//  Check the disk
		fo, err := p.LoadMetadata(bo.identifier)
		if err != nil {
			return nil, err
		} else {
			fo.Expires = bo.Expires
			err = p.SaveMetadata(*fo)
			if err != nil {
				return nil, err
			} else {
				return fo, err
			}
		}
	}
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

func (f *FileObject) Read(buf []byte) (int, error) {
	return f.File.Read(buf)
}

func (f *FileObject) Close() error {
	if f.File == nil {
		return nil
	} else {
		return f.File.Close()
	}
}
