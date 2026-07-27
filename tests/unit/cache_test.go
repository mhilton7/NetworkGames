package unit_test

import (
	"testing"

	"wiibridge/pi/controller/cache"
)

func TestCacheBoundAndSnapshotIsolation(t *testing.T) {
	c := cache.New(1024)
	for i := 0; i < 100; i++ {
		key := cache.Key{Snapshot: "one", Offset: int64(i * 256), Length: 256, Version: 1}
		c.Put(key, make([]byte, 256))
		if c.Bytes() > 1024 {
			t.Fatalf("cache exceeded bound: %d", c.Bytes())
		}
	}
	if _, ok := c.Get(cache.Key{Snapshot: "two", Offset: 99 * 256, Length: 256, Version: 1}); ok {
		t.Fatal("cache leaked across snapshot identity")
	}
	c.Clear()
	if c.Bytes() != 0 {
		t.Fatal("clear did not release cache accounting")
	}
}
