// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025 Datadog, Inc.

package internal

// Recorder is a generic thread-safe type that records functions that could have taken place before object T was created.
// Once object T is created, the Recorder can replay all the recorded functions with object T as an argument.
type Recorder[T any] struct {
	queue *RingQueue[func(T)]
}

// TODO: tweak these values once we get telemetry data from the telemetry client
var queueCap = Range[int]{
	Min: 16,  // Initial queue capacity
	Max: 512, // Maximum queue capacity
}

// NewRecorder creates a new [Recorder] instance. with 512 as the maximum number of recorded functions before overflowing.
func NewRecorder[T any]() Recorder[T] {
	return Recorder[T]{
		queue: NewRingQueue[func(T)](queueCap),
	}
}

// Record takes a function and records it in the [Recorder]'s queue. If the queue is full, it returns false.
// Once [Recorder.Replay] is called, all recorded functions will be replayed with object T as an argument in order of recording.
func (r Recorder[T]) Record(f func(T)) bool {
	if r.queue == nil {
		return true
	}
	return r.queue.Enqueue(f)
}

// Replay uses T as an argument to replay all recorded functions in order of recording.
func (r Recorder[T]) Replay(t T) {
	if r.queue == nil {
		return
	}
	for {
		f := r.queue.Dequeue()
		if f == nil {
			break
		}
		f(t)
	}
}

// Clear clears the Recorder's queue.
func (r Recorder[T]) Clear() {
	r.queue.Clear()
}
