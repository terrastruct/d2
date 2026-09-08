// Package typedpool provides a type-safe wrapper around sync.Pool.
package typedpool

import "sync"

// Pool keeps sync.Pool's allocation behavior while making both Get and Put
// type-safe at their call sites.
type Pool[T any] struct {
	pool sync.Pool
}

// New constructs a typed pool whose allocator returns a fresh T.
func New[T any](newValue func() T) *Pool[T] {
	return &Pool[T]{
		pool: sync.Pool{
			New: func() any { return newValue() },
		},
	}
}

// Get selects an arbitrary item from the pool or calls its allocator.
func (pool *Pool[T]) Get() T {
	return pool.pool.Get().(T)
}

// Put adds value to the pool.
func (pool *Pool[T]) Put(value T) {
	pool.pool.Put(value)
}
