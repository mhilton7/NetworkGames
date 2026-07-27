package nbd

import "testing"

func TestRequestBufferReuseIsBoundedAndAllocationFreeAfterWarmup(t *testing.T) {
	server := &Server{MaxRequest: 1 << 20}
	buffer := server.acquireRequestBuffer(64 << 10)
	server.releaseRequestBuffer(buffer)
	allocations := testing.AllocsPerRun(1000, func() {
		current := server.acquireRequestBuffer(64 << 10)
		current.data[0] = 1
		server.releaseRequestBuffer(current)
	})
	if allocations != 0 {
		t.Fatalf("warm request buffer allocations = %f, want 0", allocations)
	}
	oversized := server.acquireRequestBuffer(2 << 20)
	server.releaseRequestBuffer(oversized)
	reused := server.acquireRequestBuffer(64 << 10)
	defer server.releaseRequestBuffer(reused)
	if cap(reused.data) > int(server.MaxRequest) {
		t.Fatalf("oversized request buffer retained: %d", cap(reused.data))
	}
}

func BenchmarkRequestBuffer64KiB(b *testing.B) {
	server := &Server{MaxRequest: 1 << 20}
	for range b.N {
		buffer := server.acquireRequestBuffer(64 << 10)
		buffer.data[0] = 1
		benchmarkBufferSink = buffer.data
		server.releaseRequestBuffer(buffer)
	}
}

var benchmarkBufferSink []byte
