package keyboard

import (
	"testing"
	"time"
)

func TestEvent(t *testing.T) {
	now := time.Now()
	ev := Event{Code: 2, IsPress: true, Time: now}
	if ev.Code != 2 {
		t.Fatalf("Code: got %d", ev.Code)
	}
	if !ev.IsPress {
		t.Fatal("IsPress should be true")
	}
	if !ev.Time.Equal(now) {
		t.Fatal("Time mismatch")
	}
}

func TestWatchDevices(t *testing.T) {
	created, removed, err := WatchDevices(nil)
	if err != nil {
		t.Fatalf("WatchDevices: %v", err)
	}
	if created == nil || removed == nil {
		t.Fatal("WatchDevices returned nil channels")
	}
}


