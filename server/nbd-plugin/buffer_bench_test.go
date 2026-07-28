package nbd

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"wiibridge/shared/perf"
)

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

type benchmarkBackend struct{ size int64 }

func (b benchmarkBackend) Size() int64 { return b.size }
func (benchmarkBackend) ReadAt(buffer []byte, _ int64) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

type benchmarkConn struct {
	input  bytes.Reader
	output bytes.Buffer
}

func (c *benchmarkConn) Reset(frame []byte) {
	c.input.Reset(frame)
	c.output.Reset()
}
func (c *benchmarkConn) Read(buffer []byte) (int, error)  { return c.input.Read(buffer) }
func (c *benchmarkConn) Write(buffer []byte) (int, error) { return c.output.Write(buffer) }
func (*benchmarkConn) Close() error                       { return nil }
func (*benchmarkConn) LocalAddr() net.Addr                { return benchmarkAddress("local") }
func (*benchmarkConn) RemoteAddr() net.Addr               { return benchmarkAddress("remote") }
func (*benchmarkConn) SetDeadline(time.Time) error        { return nil }
func (*benchmarkConn) SetReadDeadline(time.Time) error    { return nil }
func (*benchmarkConn) SetWriteDeadline(time.Time) error   { return nil }

type benchmarkAddress string

func (a benchmarkAddress) Network() string { return "benchmark" }
func (a benchmarkAddress) String() string  { return string(a) }

func BenchmarkNBDReadPathMetrics(b *testing.B) {
	var frame bytes.Buffer
	for _, value := range []any{
		uint32(requestMagic), uint32(cmdRead), uint64(1), uint64(0), uint32(64 << 10),
	} {
		if err := binary.Write(&frame, binary.BigEndian, value); err != nil {
			b.Fatal(err)
		}
	}
	request := append([]byte(nil), frame.Bytes()...)
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		b.Run(name, func(b *testing.B) {
			server := &Server{
				MaxRequest: 1 << 20,
				Metrics:    perf.New(perf.Config{Enabled: enabled}),
			}
			conn := &benchmarkConn{}
			backend := benchmarkBackend{size: 64 << 10}
			conn.Reset(request)
			_ = server.transmission(conn, backend, true)
			b.ReportAllocs()
			b.SetBytes(64 << 10)
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				conn.Reset(request)
				if err := server.transmission(conn, backend, true); !errorsIsEOF(err) {
					b.Fatal(err)
				}
			}
		})
	}
}

func errorsIsEOF(err error) bool {
	return err == io.EOF
}
