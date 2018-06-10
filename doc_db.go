package gcse

import (
	villa "github.com/daviddengcn/go-villa"
)

type DocDB interface {
	Sync() error
	Export(root villa.Path, kind string) error

	Get(key string, data interface{}) bool
	Put(key string, data interface{})
	Delete(key string)
	Iterate(output func(key string, val interface{}) error) error
}
