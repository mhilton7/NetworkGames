package perf

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestHistogramAndCounters(t *testing.T) {
	registry := New(Config{Enabled: true, SessionHistory: true, MaxSessions: 2})
	registry.StartSession("wii", "", "host", "firmware", 1)
	registry.ObserveSourceRead(4096, time.Millisecond, nil)
	registry.ObserveSourceRead(0, 10*time.Millisecond, errors.New("offline"))
	registry.ObserveNBDRead(4096, 2*time.Millisecond, nil)
	registry.RecordUSBResets(2)
	registry.RecordSaveFlush()
	snapshot := registry.Snapshot("Ready", "ready", 10, 512<<20)
	if snapshot.Source.Counters["read_operations"] != 2 ||
		snapshot.Source.Counters["read_errors"] != 1 ||
		snapshot.NBD.Counters["read_requests"] != 1 ||
		snapshot.NBD.Latency.P95US == 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	session, ok := registry.EndSession("detached")
	if !ok || session.TotalBytes != 4096 || session.ReadCount != 1 ||
		session.SourceErrors != 1 || session.USBResets != 2 ||
		session.SaveFlushes != 1 {
		t.Fatalf("session = %#v, %v", session, ok)
	}
}

func TestSessionHistoryBounded(t *testing.T) {
	registry := New(Config{Enabled: true, SessionHistory: true, MaxSessions: 2})
	for index := 0; index < 4; index++ {
		registry.StartSession("wii", "", "host", "", 1)
		registry.EndSession("detached")
	}
	if sessions := registry.Sessions(0, 100); len(sessions) != 2 {
		t.Fatalf("sessions = %d", len(sessions))
	}
}

func TestMetricsDisabled(t *testing.T) {
	registry := New(Config{Enabled: false})
	registry.ObserveNBDRead(4096, time.Millisecond, nil)
	if registry.Snapshot("", "", 0, 0).NBD.Counters["read_requests"] != 0 {
		t.Fatal("disabled metrics changed")
	}
}

func TestRollingRatesRemainBounded(t *testing.T) {
	var rates RollingRates
	now := time.Now()
	for index := 0; index < 10000; index++ {
		rates.Add(now, 512)
	}
	snapshot := rates.Snapshot(now)
	if snapshot.OperationsPerSecond1m <= 0 || len(rates.buckets) != 300 {
		t.Fatalf("rates = %#v", snapshot)
	}
}

func TestRollingRateRolloverDoesNotDiscardConcurrentSamples(t *testing.T) {
	var rates RollingRates
	now := time.Unix(1_000_000, 0)
	rates.Add(now.Add(-300*time.Second), 1)
	const workers = 32
	const perWorker = 1000
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for range perWorker {
				rates.Add(now, 512)
			}
		}()
	}
	group.Wait()
	bucket := &rates.buckets[now.Unix()%int64(len(rates.buckets))]
	if got, want := bucket.ops.Load(), uint64(workers*perWorker); got != want {
		t.Fatalf("concurrent operations = %d, want %d", got, want)
	}
	if got, want := bucket.bytes.Load(), uint64(workers*perWorker*512); got != want {
		t.Fatalf("concurrent bytes = %d, want %d", got, want)
	}
}

func BenchmarkObserveNBDReadEnabled(b *testing.B) {
	registry := New(Config{Enabled: true})
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		registry.ObserveNBDRead(64<<10, 100*time.Microsecond, nil)
	}
}

func BenchmarkObserveNBDReadDisabled(b *testing.B) {
	registry := New(Config{Enabled: false})
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		registry.ObserveNBDRead(64<<10, 100*time.Microsecond, nil)
	}
}
