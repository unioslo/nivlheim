package main

import (
	"math"
	"sync"
	"time"
)

// IntervalBuffer is a memory structure that you can add numbers to,
// and it "forgets" them after a certain amount of time.
// This is useful for counting items per minute, producing producing moving averages, etc.
type IntervalBuffer struct {
	interval time.Duration
	buffer   []intervalBufferElement
	position int
	mu       sync.Mutex
}

type intervalBufferElement struct {
	value     float64
	timestamp time.Time
}

// NewIntervalBuffer creates a new IntervalBuffer that operates
// on the given interval of time
func NewIntervalBuffer(_interval time.Duration) *IntervalBuffer {
	return &IntervalBuffer{
		interval: _interval,
		buffer:   make([]intervalBufferElement, 0),
		position: 0,
	}
}

// Add another value to the buffer, using the current time as the timestamp
func (ma *IntervalBuffer) Add(newValue float64) {
	ma.AddT(newValue, time.Now())
}

// AddT adds another value to the buffer, with the given timestamp
func (ma *IntervalBuffer) AddT(newValue float64, when time.Time) {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	newElement := intervalBufferElement{
		value:     newValue,
		timestamp: when,
	}
	if len(ma.buffer) == 0 {
		ma.buffer = append(ma.buffer, newElement)
		return
	}
	if ma.position >= len(ma.buffer) {
		ma.position = 0
	}
	if time.Since(ma.buffer[ma.position].timestamp) > ma.interval {
		ma.buffer[ma.position] = newElement
		ma.position++
		return
	}
	ma.buffer = append(ma.buffer, newElement)
}

// Sum returns the sum of all numbers in the buffer with a timestamp
// not older than the buffer's interval
func (ma *IntervalBuffer) Sum() float64 {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	var sum float64
	now := time.Now()
	for _, elem := range ma.buffer {
		if now.Sub(elem.timestamp) <= ma.interval {
			sum += elem.value
		}
	}
	return sum
}

// Average returns the average of all numbers in the buffer with a timestamp
// not older than the buffer's interval
func (ma *IntervalBuffer) Average() float64 {
	ma.mu.Lock()
	defer ma.mu.Unlock()
	var sum float64
	count := 0
	now := time.Now()
	for _, elem := range ma.buffer {
		if now.Sub(elem.timestamp) <= ma.interval {
			sum += elem.value
			count++
		}
	}
	if count == 0 {
		return math.NaN()
	}
	return sum / float64(count)
}
