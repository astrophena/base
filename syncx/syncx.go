// © 2024 Ilya Mateyko. All rights reserved.
// Use of this source code is governed by the ISC
// license that can be found in the LICENSE.md file.

// Package syncx contains useful synchronization primitives.
package syncx

import "sync"

// Protect wraps T into [Protected].
func Protect[T any](val T) Protected[T] { return Protected[T]{val: val} }

// Protected provides synchronized access to a value of type T.
// It should not be copied.
type Protected[T any] struct {
	mu  sync.RWMutex
	val T
}

// ReadAccess provides read access to the protected value.
// It executes the provided function f with the value under a read lock.
func (p *Protected[T]) ReadAccess(f func(T)) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	f(p.val)
}

// WriteAccess provides write access to the protected value.
// It executes the provided function f with the value under a write lock.
func (p *Protected[T]) WriteAccess(f func(T)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	f(p.val)
}

// Lazy represents a lazily computed value.
type Lazy[T any] struct {
	once sync.Once
	val  T
	err  error
}

// Get returns T, calling f to compute it, if necessary.
func (l *Lazy[T]) Get(f func() T) T {
	l.once.Do(func() { l.val = f() })
	return l.val
}

// GetErr returns T and an error, calling f to compute them, if necessary.
func (l *Lazy[T]) GetErr(f func() (T, error)) (T, error) {
	l.once.Do(func() { l.val, l.err = f() })
	return l.val, l.err
}

// LimitedWaitGroup is a [sync.WaitGroup] that limits the number of concurrently
// working goroutines.
type LimitedWaitGroup struct {
	wg      sync.WaitGroup
	workers chan struct{}
}

// NewLimitedWaitGroup returns a new [LimitedWaitGroup].
func NewLimitedWaitGroup(limit int) *LimitedWaitGroup {
	return &LimitedWaitGroup{
		workers: make(chan struct{}, limit),
	}
}

// Go starts a new goroutine that executes f.
// It blocks if the number of active goroutines reaches the concurrency limit.
func (lwg *LimitedWaitGroup) Go(f func()) {
	lwg.Add(1)
	go func() {
		defer lwg.Done()
		f()
	}()
}

// Add increments the counter of the [LimitedWaitGroup] by the specified delta.
// It blocks if the number of active goroutines reaches the concurrency limit.
func (lwg *LimitedWaitGroup) Add(delta int) {
	for range delta {
		lwg.workers <- struct{}{}
		lwg.wg.Add(1)
	}
}

// Done decrements the counter of the [LimitedWaitGroup] by one and releases a
// slot, allowing another goroutine to start.
func (lwg *LimitedWaitGroup) Done() {
	<-lwg.workers
	lwg.wg.Done()
}

// Wait blocks until the counter of the [LimitedWaitGroup] becomes zero.
func (lwg *LimitedWaitGroup) Wait() { lwg.wg.Wait() }

// Map is a generic version of [sync.Map].
type Map[K comparable, V any] struct{ m sync.Map }

// Load is [sync.Map.Load].
func (m *Map[K, V]) Load(key K) (value V, ok bool) {
	val, ok := m.m.Load(key)
	if !ok {
		return value, false
	}
	return val.(V), true
}

// Store is [sync.Map.Store].
func (m *Map[K, V]) Store(key K, value V) { m.m.Store(key, value) }

// LoadOrStore is [sync.Map.LoadOrStore].
func (m *Map[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	v, loaded := m.m.LoadOrStore(key, value)
	return v.(V), loaded
}

// LoadAndDelete is [sync.Map.LoadAndDelete].
func (m *Map[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	v, loaded := m.m.LoadAndDelete(key)
	return v.(V), loaded
}

// Delete is [sync.Map.Delete].
func (m *Map[K, V]) Delete(key K) { m.m.Delete(key) }

// Range is [sync.Map.Range].
func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(key, value any) bool {
		return f(key.(K), value.(V))
	})
}
