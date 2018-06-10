package gcse

import (
	"encoding/gob"
	"log"

	"github.com/golangplus/bytes"
)

type PackedDocDB struct {
	*MemDB
}

func (db PackedDocDB) Get(key string, data interface{}) bool {
	var bs bytesp.Slice
	if ok := db.MemDB.Get(key, (*[]byte)(&bs)); !ok {
		return false
	}
	dec := gob.NewDecoder(&bs)
	if err := dec.Decode(data); err != nil {
		log.Printf("Get %s failed: %v", key, err)
		return false
	}
	return true
}

func (db PackedDocDB) Put(key string, data interface{}) {
	var bs bytesp.Slice
	enc := gob.NewEncoder(&bs)
	if err := enc.Encode(data); err != nil {
		log.Printf("Put %s failed: %v", key, err)
		return
	}
	db.MemDB.Put(key, []byte(bs))
}

func (db PackedDocDB) Iterate(
	output func(key string, val interface{}) error) error {
	return db.MemDB.Iterate(func(key string, val interface{}) error {
		dec := gob.NewDecoder(bytesp.NewPSlice(val.([]byte)))
		var info DocInfo
		if err := dec.Decode(&info); err != nil {
			log.Printf("Decode %s failed: %v", key, err)
			db.Get(key, &info)
			return err
		}
		return output(key, info)
	})
}
