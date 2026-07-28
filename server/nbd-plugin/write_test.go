package nbd

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

type protocolWriteBackend struct {
	data      []byte
	writeErr  error
	syncCount int
}

func (b *protocolWriteBackend) Size() int64 { return int64(len(b.data)) }
func (b *protocolWriteBackend) ReadAt(buffer []byte, offset int64) (int, error) {
	return copy(buffer, b.data[offset:]), nil
}
func (b *protocolWriteBackend) WriteAt(buffer []byte, offset int64) (int, error) {
	if b.writeErr != nil {
		return 0, b.writeErr
	}
	return copy(b.data[offset:], buffer), nil
}
func (b *protocolWriteBackend) Sync() error {
	b.syncCount++
	return nil
}

type codedWriteError struct{ code string }

func (e codedWriteError) Error() string     { return e.code }
func (e codedWriteError) ErrorCode() string { return e.code }

func writeRequestFrame(t *testing.T, offset uint64, payload []byte) []byte {
	t.Helper()
	var frame bytes.Buffer
	for _, value := range []any{
		uint32(requestMagic), uint32(cmdWrite), uint64(7), offset, uint32(len(payload)),
	} {
		if err := binary.Write(&frame, binary.BigEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	frame.Write(payload)
	return frame.Bytes()
}

func replyStatus(t *testing.T, output []byte) uint32 {
	t.Helper()
	if len(output) != 16 {
		t.Fatalf("reply length=%d", len(output))
	}
	if magic := binary.BigEndian.Uint32(output[:4]); magic != replyMagic {
		t.Fatalf("reply magic=%x", magic)
	}
	return binary.BigEndian.Uint32(output[4:8])
}

func TestNBDWriteRoutesOnlyWritableBackendAndReturnsPermissionForExtentError(t *testing.T) {
	server := &Server{MaxRequest: 1 << 20}
	payload := []byte("bounded-save")
	for _, test := range []struct {
		name     string
		readOnly bool
		writeErr error
		want     uint32
		written  bool
	}{
		{name: "valid", want: 0, written: true},
		{name: "read-only", readOnly: true, want: errPerm},
		{name: "outside extent", writeErr: codedWriteError{
			code: "SAVE-WRITE-OUTSIDE-EXTENT",
		}, want: errPerm},
		{name: "crosses extent", writeErr: codedWriteError{
			code: "SAVE-WRITE-CROSSES-BOUNDARY",
		}, want: errPerm},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend := &protocolWriteBackend{
				data: make([]byte, 4096), writeErr: test.writeErr,
			}
			conn := &benchmarkConn{}
			conn.Reset(writeRequestFrame(t, 512, payload))
			if err := server.transmission(conn, backend, test.readOnly); err != io.EOF {
				t.Fatalf("transmission=%v", err)
			}
			if status := replyStatus(t, conn.output.Bytes()); status != test.want {
				t.Fatalf("status=%d want=%d", status, test.want)
			}
			if got := bytes.Equal(backend.data[512:512+len(payload)], payload); got != test.written {
				t.Fatalf("written=%t want=%t", got, test.written)
			}
		})
	}
}

func TestNBDFlushReachesWritableSaveBackend(t *testing.T) {
	var frame bytes.Buffer
	for _, value := range []any{
		uint32(requestMagic), uint32(cmdFlush), uint64(8), uint64(0), uint32(0),
	} {
		if err := binary.Write(&frame, binary.BigEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	backend := &protocolWriteBackend{data: make([]byte, 4096)}
	conn := &benchmarkConn{}
	conn.Reset(frame.Bytes())
	server := &Server{MaxRequest: 1 << 20}
	if err := server.transmission(conn, backend, false); err != io.EOF {
		t.Fatal(err)
	}
	if backend.syncCount != 1 || replyStatus(t, conn.output.Bytes()) != 0 {
		t.Fatalf("syncs=%d reply=%x", backend.syncCount, conn.output.Bytes())
	}
}
