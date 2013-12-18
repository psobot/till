package main

import (
	"io"
)

type Object interface {
	URL() *string
	GetBaseObject() BaseObject

	GetSize() (int64, error)
	Read(length int) ([]byte, error)
	Close()
}

type BaseObject struct {
	// JSON-Serializable (a.k.a: stored on disk) metadata
	Expires  int64  `json:"expires"`
	Metadata string `json:"metadata"`

	identifier string
	exists     bool
	provider   Provider
}

func (b BaseObject) GetBaseObject() BaseObject {
	return b
}

func (b *BaseObject) URL() *string {
	if b.provider == nil {
		return nil
	} else {
		//  TODO
		return nil
	}
}

func (b *BaseObject) GetProvider() Provider {
	return b.provider
}

func (b *BaseObject) GetSize() (int64, error) {
	return -1, nil
}

func (b *BaseObject) Read(length int) ([]byte, error) {
	return []byte{}, nil
}

func (b *BaseObject) Close() {

}

type UploadObject struct {
	BaseObject

	reader io.ReadCloser
	size   int64
}

func (b *UploadObject) GetSize() (int64, error) {
	return b.size, nil
}

func (b *UploadObject) Read(length int) ([]byte, error) {
	buf := make([]byte, length)
	len, err := b.reader.Read(buf)
	if err != nil {
		return []byte{}, err
	} else {
		return buf[:len], nil
	}
}