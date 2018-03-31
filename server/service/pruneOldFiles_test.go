package main

import (
	"math/rand"
	"testing"
	"time"
)

func TestGenerateTimeTable(t *testing.T) {
	table := pruneTimeTable
	// how many slots <= 30 days
	count := 0
	for i, slot := range table {
		if slot <= time.Duration(30*24)*time.Hour {
			count++
			if i > 0 {
				diff := table[i] - table[i-1]
				if diff != time.Duration(24)*time.Hour {
					t.Errorf("Difference between slots is %v", diff)
				}
			}
		}
	}
	if count != 30 {
		t.Errorf("Wrong number of 30 day slots, got %d", count)
	}
	// how many slots <= (52 weeks and 30 days)
	count = 0
	for _, slot := range table {
		if slot <= time.Duration((52*7+30)*24)*time.Hour {
			count++
		}
	}
	if count != 30+52 {
		t.Errorf("Wrong number of one year slots, got %d", count)
	}
}

func TestWhatToDelete(t *testing.T) {
	data := make(map[int64]time.Time)
	// add a bunch of stuff <24h
	const dayItems = 100
	var fileID int64 = 1
	var maxSeconds = 24 * 60 * 60
	for i := 0; i < dayItems; i++ {
		data[fileID] = time.Now().Add(-time.Duration(rand.Intn(maxSeconds)) * time.Second)
		fileID++
	}
	del := whatToDelete(&data)
	if len(del) > 0 {
		t.Errorf("Shouldn't delete anything younger than 24 hours.")
	}

	// now try things between 24 hours and 30 days
	data = make(map[int64]time.Time)
	minSeconds := 60 * 60 * 24
	maxSeconds = minSeconds + 60*60*24*29 - 1
	for i := 0; i < 1000; i++ {
		data[fileID] = time.Now().Add(-time.Duration(minSeconds+
			rand.Intn(maxSeconds-minSeconds)) * time.Second)
		fileID++
	}

	// the newest(latest) and the oldest(earliest) version
	// should be kept regardless
	type record struct {
		fileID int64
		mtime  time.Time
	}
	var oldest, newest record
	for i, t := range data {
		if oldest.fileID == 0 || oldest.mtime.After(t) {
			oldest.fileID = i
			oldest.mtime = t
		}
		if newest.fileID == 0 || newest.mtime.Before(t) {
			newest.fileID = i
			newest.mtime = t
		}
	}

	del = whatToDelete(&data)
	for _, id := range del {
		if id == newest.fileID {
			t.Errorf("The newest record got deleted")
		} else if id == oldest.fileID {
			t.Errorf("The oldest record got deleted")
		}
		delete(data, id)
	}
	if len(data) != 30 {
		t.Errorf("Should have 30 items for last month, had %d", len(data))
		t.Errorf("Newest: %v\nOldest: %v", newest.mtime, oldest.mtime)
	}
}
