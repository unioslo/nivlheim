package main

import (
	"math"
	"testing"
	"time"
)

func TestIntervalBuffer(t *testing.T) {
	b := NewIntervalBuffer(time.Minute)
	for i := 500; i >= 0; i-- {
		b.AddT(1, time.Now().Add(-time.Duration(i)*time.Second))
	}
	f := b.Sum()
	if math.Abs(f-60) > 0.00001 {
		t.Errorf("Sum() = %f, expected %f", f, 60)
	}
	if len(b.buffer) != 60 {
		t.Errorf("Buffer length = %d, expected 60", len(b.buffer))
	}
}
