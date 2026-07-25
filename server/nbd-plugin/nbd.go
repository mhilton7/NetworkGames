// Package nbd implements the minimum fixed-newstyle NBD protocol required for
// a read-only export. It requires NBD_OPT_STARTTLS before export selection.
package nbd

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

const (
	nbdMagic       = uint64(0x4e42444d41474943)
	optMagic       = uint64(0x49484156454f5054)
	repMagic       = uint64(0x0003e889045565a9)
	requestMagic   = uint32(0x25609513)
	replyMagic     = uint32(0x67446698)
	optExportName  = uint32(1)
	optAbort       = uint32(2)
	optStartTLS    = uint32(5)
	optInfo        = uint32(6)
	optGo          = uint32(7)
	repAck         = uint32(1)
	repInfo        = uint32(3)
	repErrUnsup    = uint32(0x80000001)
	cmdRead        = uint16(0)
	cmdWrite       = uint16(1)
	cmdDisconnect  = uint16(2)
	cmdFlush       = uint16(3)
	cmdTrim        = uint16(4)
	cmdWriteZeroes = uint16(6)
	flagFixedNew   = uint16(1)
	flagNoZeroes   = uint16(2)
	flagHasFlags   = uint16(1)
	flagReadOnly   = uint16(2)
	flagSendFlush  = uint16(4)
	errPerm        = uint32(1)
	errIO          = uint32(5)
	errInvalid     = uint32(22)
)

type Backend interface {
	Size() int64
	ReadAt([]byte, int64) (int, error)
}

type Server struct {
	Backend Backend // fixed backend, primarily for tests and single snapshots
	// BackendProvider is called once per connection.  The returned immutable
	// backend remains pinned for the complete NBD session.
	BackendProvider func() Backend
	TLS             *tls.Config
	ExportName      string
	Deadline        time.Duration
	MaxRequest      uint32
	Logger          *slog.Logger
	mu              sync.Mutex
	active          map[net.Conn]struct{}
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if (s.Backend == nil && s.BackendProvider == nil) ||
		s.TLS == nil || s.TLS.ClientAuth != tls.RequireAndVerifyClientCert {
		return errors.New("backend and mutual TLS are required")
	}
	if s.Deadline == 0 {
		s.Deadline = 30 * time.Second
	}
	if s.MaxRequest == 0 {
		s.MaxRequest = 1 << 20
	}
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	s.active = make(map[net.Conn]struct{})
	go func() {
		<-ctx.Done()
		listener.Close()
		s.mu.Lock()
		defer s.mu.Unlock()
		for c := range s.active {
			c.Close()
		}
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		s.mu.Lock()
		s.active[conn] = struct{}{}
		s.mu.Unlock()
		go func() {
			defer func() {
				conn.Close()
				s.mu.Lock()
				delete(s.active, conn)
				s.mu.Unlock()
			}()
			if err := s.handle(conn); err != nil && !errors.Is(err, io.EOF) {
				s.Logger.Warn("NBD session closed", "error", err)
			}
		}()
	}
}

func (s *Server) handle(raw net.Conn) error {
	backend := s.Backend
	if s.BackendProvider != nil {
		backend = s.BackendProvider()
	}
	if backend == nil {
		return errors.New("no published backend")
	}
	raw.SetDeadline(time.Now().Add(s.Deadline))
	if err := binary.Write(raw, binary.BigEndian, nbdMagic); err != nil {
		return err
	}
	if err := binary.Write(raw, binary.BigEndian, optMagic); err != nil {
		return err
	}
	if err := binary.Write(raw, binary.BigEndian, flagFixedNew|flagNoZeroes); err != nil {
		return err
	}
	var clientFlags uint32
	if err := binary.Read(raw, binary.BigEndian, &clientFlags); err != nil {
		return err
	}
	if clientFlags&1 == 0 {
		return errors.New("client did not request fixed-newstyle")
	}
	var conn net.Conn = raw
	secured := false
	for {
		var magic uint64
		var option, length uint32
		if err := binary.Read(conn, binary.BigEndian, &magic); err != nil {
			return err
		}
		if magic != optMagic {
			return errors.New("invalid option magic")
		}
		if err := binary.Read(conn, binary.BigEndian, &option); err != nil {
			return err
		}
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return err
		}
		if length > 64<<10 {
			return errors.New("option too large")
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return err
		}
		if !secured {
			if option != optStartTLS || length != 0 {
				if err := writeOptionReply(conn, option, repErrUnsup, nil); err != nil {
					return err
				}
				continue
			}
			if err := writeOptionReply(conn, option, repAck, nil); err != nil {
				return err
			}
			tlsConn := tls.Server(raw, s.TLS.Clone())
			if err := tlsConn.Handshake(); err != nil {
				return err
			}
			conn, secured = tlsConn, true
			continue
		}
		switch option {
		case optAbort:
			_ = writeOptionReply(conn, option, repAck, nil)
			return nil
		case optExportName:
			if string(payload) != s.ExportName {
				return errors.New("unknown export")
			}
			if err := binary.Write(conn, binary.BigEndian, uint64(backend.Size())); err != nil {
				return err
			}
			if err := binary.Write(conn, binary.BigEndian, flagHasFlags|flagReadOnly|flagSendFlush); err != nil {
				return err
			}
			return s.transmission(conn, backend)
		case optInfo, optGo:
			name, ok := parseInfoName(payload)
			if !ok || name != s.ExportName {
				if err := writeOptionReply(conn, option, repErrUnsup, nil); err != nil {
					return err
				}
				continue
			}
			info := make([]byte, 12)
			binary.BigEndian.PutUint16(info[0:2], 0)
			binary.BigEndian.PutUint64(info[2:10], uint64(backend.Size()))
			binary.BigEndian.PutUint16(info[10:12], flagHasFlags|flagReadOnly|flagSendFlush)
			if err := writeOptionReply(conn, option, repInfo, info); err != nil {
				return err
			}
			block := make([]byte, 14)
			binary.BigEndian.PutUint16(block[0:2], 3)
			binary.BigEndian.PutUint32(block[2:6], 512)
			binary.BigEndian.PutUint32(block[6:10], 64<<10)
			binary.BigEndian.PutUint32(block[10:14], s.MaxRequest)
			if err := writeOptionReply(conn, option, repInfo, block); err != nil {
				return err
			}
			if err := writeOptionReply(conn, option, repAck, nil); err != nil {
				return err
			}
			if option == optGo {
				return s.transmission(conn, backend)
			}
		default:
			if err := writeOptionReply(conn, option, repErrUnsup, nil); err != nil {
				return err
			}
		}
	}
}

func parseInfoName(payload []byte) (string, bool) {
	if len(payload) < 4 {
		return "", false
	}
	n := int(binary.BigEndian.Uint32(payload[:4]))
	if n < 0 || len(payload) < 4+n+2 {
		return "", false
	}
	return string(payload[4 : 4+n]), true
}

func writeOptionReply(w io.Writer, option, reply uint32, payload []byte) error {
	var frame bytes.Buffer
	for _, v := range []any{repMagic, option, reply, uint32(len(payload))} {
		if err := binary.Write(&frame, binary.BigEndian, v); err != nil {
			return err
		}
	}
	frame.Write(payload)
	_, err := w.Write(frame.Bytes())
	return err
}

func (s *Server) transmission(conn net.Conn, backend Backend) error {
	// A connected block device may legitimately be idle for an arbitrary
	// period. The negotiation deadline protects the unauthenticated and TLS
	// setup phases, but carrying it into transmission tears down healthy
	// mounted devices when no read arrives before the deadline.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return err
	}
	for {
		var magic, flagsType uint32
		var handle, offset uint64
		var length uint32
		for _, dst := range []any{&magic, &flagsType, &handle, &offset, &length} {
			if err := binary.Read(conn, binary.BigEndian, dst); err != nil {
				return err
			}
		}
		if magic != requestMagic {
			return errors.New("invalid request magic")
		}
		command := uint16(flagsType)
		if command == cmdDisconnect {
			return nil
		}
		status := uint32(0)
		var data []byte
		switch command {
		case cmdRead:
			if length == 0 || length > s.MaxRequest || offset > uint64(backend.Size()) ||
				uint64(length) > uint64(backend.Size())-offset {
				status = errInvalid
			} else {
				data = make([]byte, length)
				if _, err := backend.ReadAt(data, int64(offset)); err != nil {
					status, data = errIO, nil
				}
			}
		case cmdFlush:
			// Immutable backend: a flush is a safe no-op.
		case cmdWrite, cmdTrim, cmdWriteZeroes:
			status = errPerm
			if command == cmdWrite && length <= s.MaxRequest {
				if _, err := io.CopyN(io.Discard, conn, int64(length)); err != nil {
					return err
				}
			}
		default:
			status = errInvalid
		}
		for _, v := range []any{replyMagic, status, handle} {
			if err := binary.Write(conn, binary.BigEndian, v); err != nil {
				return err
			}
		}
		if status == 0 && command == cmdRead {
			if _, err := conn.Write(data); err != nil {
				return err
			}
		}
	}
}
