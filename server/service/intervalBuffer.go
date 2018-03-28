package main

import (
	"sync"
	"time"
)

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

func NewIntervalBuffer(_interval time.Duration) *IntervalBuffer {
	return &IntervalBuffer{
		interval: _interval,
		buffer:   make([]intervalBufferElement, 0),
		position: 0,
	}
}

func (ma *IntervalBuffer) Add(newValue float64) {
	ma.AddT(newValue, time.Now())
}

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
