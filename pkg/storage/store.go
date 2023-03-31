package storage

import (
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/rubble/pkg/log"
	"os"
	"path/filepath"
	"sync"
)

var ErrNotFound = fmt.Errorf("not found")
var logger = log.DefaultLogger.WithField("component:", "rubble storage")

type Storage interface {
	Put(key string, value interface{}) error
	Get(key string) (interface{}, error)
	List() ([]interface{}, error)
	Delete(key string) error
}

type MemoryStorage struct {
	lock  sync.RWMutex
	store map[string]interface{}
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		store: make(map[string]interface{}),
	}
}

func (m *MemoryStorage) Put(key string, value interface{}) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.store[key] = value
	return nil
}

func (m *MemoryStorage) Get(key string) (interface{}, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	value, ok := m.store[key]
	if !ok {
		return nil, ErrNotFound
	}
	return value, nil
}

func (m *MemoryStorage) List() ([]interface{}, error) {
	m.lock.RLock()
	defer m.lock.RUnlock()
	var ret []interface{}
	for _, v := range m.store {
		ret = append(ret, v)
	}
	return ret, nil
}

func (m *MemoryStorage) Delete(key string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.store, key)
	return nil
}

type Serializer func(interface{}) ([]byte, error)
type Deserializer func([]byte) (interface{}, error)

type DiskStorage struct {
	db           *bolt.DB
	name         string
	memory       *MemoryStorage
	serializer   Serializer
	deserializer Deserializer
}

func NewDiskStorage(name string, path string, serializer Serializer, deserializer Deserializer) (Storage, error) {
	dirPath := filepath.Dir(path)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		if err = os.MkdirAll(dirPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create dir:%s for disk storage with error:%w", dirPath, err)
		}
	}

	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, err
	}

	diskstorage := &DiskStorage{
		db:           db,
		name:         name,
		memory:       NewMemoryStorage(),
		serializer:   serializer,
		deserializer: deserializer,
	}

	err = diskstorage.load()

	if err != nil {
		return nil, err
	}
	return diskstorage, nil
}

func (d *DiskStorage) Put(key string, value interface{}) error {
	data, err := d.serializer(value)
	if err != nil {
		return err
	}

	err = d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(d.name))
		return b.Put([]byte(key), data)
	})
	if err != nil {
		return err
	}
	return d.memory.Put(key, value)
}

//load all data from disk db
func (d *DiskStorage) load() error {
	err := d.db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(d.name))
		return err
	})
	if err != nil {
		return err
	}

	err = d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(d.name))
		cursor := b.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			logger.Infof("load pod cache %s from db", k)
			obj, err := d.deserializer(v)
			if err != nil {
				return err
			}
			d.memory.Put(string(k), obj)
		}
		return nil
	})
	return err
}

func (d *DiskStorage) Get(key string) (interface{}, error) {
	return d.memory.Get(key)
}

func (d *DiskStorage) List() ([]interface{}, error) {
	return d.memory.List()
}

func (d *DiskStorage) Delete(key string) error {
	err := d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(d.name))
		return b.Delete([]byte(key))
	})
	if err != nil {
		return err
	}
	return d.memory.Delete(key)
}
