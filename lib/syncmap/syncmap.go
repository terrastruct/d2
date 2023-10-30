package syncmap

import "sync"

type SyncMap[K comparable, V any] struct {
	_map *sync.Map
}

func New[K comparable, V any]() SyncMap[K, V] {
	return SyncMap[K, V]{
		_map: &sync.Map{},
	}
}

func (sm SyncMap[K, V]) Set(key K, value V) {
	sm._map.Store(key, value)
}

func (sm SyncMap[K, V]) Lookup(key K) (value V, ok bool) {
	v, has := sm._map.Load(key)
	if !has {
		return value, false
	}
	return v.(V), true
}

func (sm SyncMap[K, V]) Get(key K) (value V) {
	v, _ := sm.Lookup(key)
	return v
}

func (sm SyncMap[K, V]) Delete(key K) {
	sm._map.Delete(key)
}

func (sm SyncMap[K, V]) Range(f func(key K, value V) bool) {
	sm._map.Range(func(k, v any) bool {
		return f(k.(K), v.(V))
	})
}
