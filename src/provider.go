package main

import (
	"errors"
)

type Provider interface {
	//  Called when instantiated and about to be destroyed.
	Connect() error
	Disconnect()

	//  API Methods
	//      The return values of each of these methods:
	//          *Object is:
	//              a pointer to the object returned
	//              OR nil if the object was not found
	//          error is:
	//              nil if the request completed
	//              non-nil if the request could not be completed
	//              (nil if the request completed, but the object was not found)

	Get(id string) (Object, error)
	GetURL(id string) (Object, error)

	Put(object Object) (Object, error)
	Update(object Object) (Object, error)
}

type BaseProvider struct {
	config ProviderConfig
}

func (b *BaseProvider) String() string {
	return "{" + b.config.Type() + " provider: " + b.config.Name() + "}"
}

func (b *BaseProvider) GetConfig() ProviderConfig {
	return b.config
}

func (b *BaseProvider) Connect() error {
	return nil
}

func (b *BaseProvider) Disconnect() {

}

func (b *BaseProvider) CanAccept(object Object) (bool, error) {
	return false, errors.New("CanAccept not implemented.")
}
