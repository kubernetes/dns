// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package tracer

import (
	"io"
	"sync"
)

// payloadStats contains the statistics of a payload.
type payloadStats struct {
	size      int // size in bytes
	itemCount int // number of items (traces)
}

// payloadWriter defines the interface for writing data to a payload.
type payloadWriter interface {
	io.Writer

	push(t spanList) (stats payloadStats, err error)
	grow(n int)
	reset()
	clear()

	// recordItem records that an item was added and updates the header
	recordItem()
}

// payloadReader defines the interface for reading data from a payload.
type payloadReader interface {
	io.Reader
	io.Closer

	stats() payloadStats
	size() int
	itemCount() int
	protocol() float64
}

// payload combines both reading and writing operations for a payload.
type payload interface {
	payloadWriter
	payloadReader
}

// newPayload returns a ready to use payload.
func newPayload(protocol float64) payload {
	if protocol == traceProtocolV1 {
		return &safePayload{
			p: newPayloadV1(),
		}
	}
	return &safePayload{
		p: newPayloadV04(),
	}
}

// https://github.com/msgpack/msgpack/blob/master/spec.md#array-format-family
const (
	// arrays
	msgpackArrayFix byte = 144  // up to 15 items
	msgpackArray16  byte = 0xdc // up to 2^16-1 items, followed by size in 2 bytes
	msgpackArray32  byte = 0xdd // up to 2^32-1 items, followed by size in 4 bytes

	// maps
	msgpackMapFix byte = 0x80 // up to 15 items
	msgpackMap16  byte = 0xde // up to 2^16-1 items, followed by size in 2 bytes
	msgpackMap32  byte = 0xdf // up to 2^32-1 items, followed by size in 4 bytes
)

// safePayload provides a thread-safe wrapper around payload.
type safePayload struct {
	mu sync.RWMutex
	p  payload
}

// push pushes a new item into the stream in a thread-safe manner.
func (sp *safePayload) push(t spanList) (stats payloadStats, err error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.p.push(t)
}

// itemCount returns the number of items available in the stream in a thread-safe manner.
// This method is not thread-safe, but the underlying payload.itemCount() must be.
func (sp *safePayload) itemCount() int {
	return sp.p.itemCount()
}

// size returns the payload size in bytes in a thread-safe manner.
func (sp *safePayload) size() int {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return sp.p.size()
}

// reset sets up the payload to be read a second time in a thread-safe manner.
func (sp *safePayload) reset() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.p.reset()
}

// clear empties the payload buffers in a thread-safe manner.
func (sp *safePayload) clear() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.p.clear()
}

// Read implements io.Reader in a thread-safe manner.
func (sp *safePayload) Read(b []byte) (n int, err error) {
	// Note: Read modifies internal state (off, reader), so we need full lock
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.p.Read(b)
}

// Close implements io.Closer in a thread-safe manner.
func (sp *safePayload) Close() error {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.p.Close()
}

// Write implements io.Writer in a thread-safe manner.
func (sp *safePayload) Write(data []byte) (n int, err error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	return sp.p.Write(data)
}

// grow grows the buffer to ensure it can accommodate n more bytes in a thread-safe manner.
func (sp *safePayload) grow(n int) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.p.grow(n)
}

// recordItem records that an item was added and updates the header in a thread-safe manner.
func (sp *safePayload) recordItem() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.p.recordItem()
}

// stats returns the current stats of the payload in a thread-safe manner.
func (sp *safePayload) stats() payloadStats {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return sp.p.stats()
}

// protocol returns the protocol version of the payload in a thread-safe manner.
func (sp *safePayload) protocol() float64 {
	// Protocol is immutable after creation - no lock needed
	return sp.p.protocol()
}
