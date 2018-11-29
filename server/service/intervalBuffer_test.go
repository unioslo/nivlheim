package main

import (
	"math"
	"testing"
	"time"
)

func TestIntervalBuffer(t *testing.T) {
	b := NewIntervalBuffer(time.Minute)
	for i := 500; i >= 0; i-- {
		b.AddT(float64(i), time.Now().Add(-time.Duration(i)*time.Second))
	}
	f := b.Sum()
	var e float64 = 1770
	if math.Abs(f-e) > 0.00001 {
		t.Errorf("Sum() = %f, expected %f", f, e)
	}
	f = b.Average()
	e = 29.5
	if math.Abs(f-e) > 0.00001 {
		t.Errorf("Sum() = %f, expected %f", f, e)
	}
	if len(b.buffer) != 60 {
		t.Errorf("Buffer length = %d, expected 60", len(b.buffer))
	}
}
