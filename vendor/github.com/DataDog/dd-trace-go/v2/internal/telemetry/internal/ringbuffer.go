// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024 Datadog, Inc.

package internal

import (
	"sync"
)

// RingQueue is a thread-safe ring buffer can be used to store a fixed number of elements and overwrite old values when full.
type RingQueue[T any] struct {
	buffer            []T
	head, tail, count int
	// mu is the lock for the buffer, head and tail.
	mu sync.Mutex
	// pool is the pool of buffers. Normally there should only be one or 2 buffers in the pool.
	pool *SyncPool[[]T]
	// BufferSizes is the range of buffer sizes that the ring queue can have.
	BufferSizes Range[int]
}

// NewRingQueue creates a new RingQueue with a minimum size and a maximum size.
func NewRingQueue[T any](rang Range[int]) *RingQueue[T] {
	return &RingQueue[T]{
		buffer:      make([]T, rang.Min),
		pool:        NewSyncPool[[]T](func() []T { return make([]T, rang.Min) }),
		BufferSizes: rang,
	}
}

// NewRingQueueWithPool creates a new RingQueue with a minimum size, a maximum size and a pool. Make sure the pool is properly initialized with the right type
func NewRingQueueWithPool[T any](rang Range[int], pool *SyncPool[[]T]) *RingQueue[T] {
	return &RingQueue[T]{
		buffer:      make([]T, rang.Min),
		pool:        pool,
		BufferSizes: rang,
	}
}

// Length returns the number of elements currently stored in the queue.
func (rq *RingQueue[T]) Length() int {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	return rq.count
}

func (rq *RingQueue[T]) resizeLocked() {
	newBuf := make([]T, rq.BufferSizes.Clamp(rq.count*2))
	defer rq.releaseBuffer(rq.buffer)

	if rq.tail > rq.head {
		copy(newBuf, rq.buffer[rq.head:rq.tail])
	} else {
		n := copy(newBuf, rq.buffer[rq.head:])
		copy(newBuf[n:], rq.buffer[:rq.tail])
	}

	rq.head = 0
	rq.tail = rq.count
	rq.buffer = newBuf
}

func (rq *RingQueue[T]) enqueueLocked(elem T) bool {
	spaceLeft := true
	if rq.count == len(rq.buffer) {
		if len(rq.buffer) == rq.BufferSizes.Max {
			spaceLeft = false
			// bitwise modulus
			rq.head = (rq.head + 1) % len(rq.buffer)
			rq.count--
		} else {
			rq.resizeLocked()
		}
	}

	rq.buffer[rq.tail] = elem
	rq.tail = (rq.tail + 1) % len(rq.buffer)
	rq.count++
	return spaceLeft
}

// ReversePeek returns the last element that was enqueued without removing it.
func (rq *RingQueue[T]) ReversePeek() T {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	if rq.count == 0 {
		var zero T
		return zero
	}
	return rq.buffer[(rq.tail-1+len(rq.buffer))%len(rq.buffer)]
}

// Enqueue adds one or multiple values to the buffer. Returns false if at least one item had to be pulled out from the queue to make space for new ones
func (rq *RingQueue[T]) Enqueue(vals ...T) bool {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	if len(vals) == 0 {
		return true
	}

	spaceLeft := true
	for _, val := range vals {
		spaceLeft = rq.enqueueLocked(val)
	}
	return spaceLeft
}

// Dequeue removes a value from the buffer.
func (rq *RingQueue[T]) Dequeue() T {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	if rq.count == 0 {
		var zero T
		return zero
	}

	ret := rq.buffer[rq.head]
	// bitwise modulus
	rq.head = (rq.head + 1) % len(rq.buffer)
	rq.count--
	return ret
}

// getBuffer returns the current buffer and resets it.
func (rq *RingQueue[T]) getBuffer() []T {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	return rq.getBufferLocked()
}

func (rq *RingQueue[T]) getBufferLocked() []T {
	prevBuf := rq.buffer
	rq.buffer = rq.pool.Get()
	rq.head, rq.tail, rq.count = 0, 0, 0
	return prevBuf
}

// Flush returns a copy of the buffer and resets it.
func (rq *RingQueue[T]) Flush() []T {
	rq.mu.Lock()
	head, count := rq.head, rq.count
	buf := rq.getBufferLocked()
	rq.mu.Unlock()

	// If the buffer is less than 12.5% full, we let the buffer get garbage collected because it's too big for the current throughput.
	// Except when the buffer is at its minimum size.
	if len(buf) == rq.BufferSizes.Min || count*8 >= len(buf) {
		defer rq.releaseBuffer(buf)
	}

	copyBuf := make([]T, count)
	for i := 0; i < count; i++ {
		copyBuf[i] = buf[(head+i)%len(buf)]
	}

	return copyBuf
}

// releaseBuffer returns the buffer to the pool.
func (rq *RingQueue[T]) releaseBuffer(buf []T) {
	var zero T
	buf = buf[:cap(buf)] // Make sure nobody reduced the length of the buffer
	for i := range buf {
		buf[i] = zero
	}
	rq.pool.Put(buf)
}

// IsEmpty returns true if the buffer is empty.
func (rq *RingQueue[T]) IsEmpty() bool {
	return rq.Length() == 0
}

// IsFull returns true if the buffer is full and cannot accept more elements.
func (rq *RingQueue[T]) IsFull() bool {
	rq.mu.Lock()
	defer rq.mu.Unlock()

	return len(rq.buffer) == rq.count && len(rq.buffer) == rq.BufferSizes.Max
}

// Clear removes all elements from the buffer.
func (rq *RingQueue[T]) Clear() {
	rq.mu.Lock()
	defer rq.mu.Unlock()
	rq.head, rq.tail, rq.count = 0, 0, 0
	var zero T
	for i := range rq.buffer {
		rq.buffer[i] = zero
	}
}
