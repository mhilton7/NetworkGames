package testutil

import (
	"encoding/binary"
	"os"
)

// SyntheticWBFS writes a minimal structurally valid, non-copyrighted fixture.
func SyntheticWBFS(path, id, title string, size int64) error {
	if size < 2<<20 {
		size = 2 << 20
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 512)
	copy(header[:4], "WBFS")
	binary.BigEndian.PutUint32(header[4:8], uint32(size/512))
	header[8], header[9], header[12] = 9, 20, 1
	if _, err = f.Write(header); err != nil {
		return err
	}
	disc := make([]byte, 0x60)
	copy(disc[:6], id)
	copy(disc[0x20:], title)
	if _, err = f.WriteAt(disc, 1<<20); err != nil {
		return err
	}
	if err = f.Truncate(size); err != nil {
		return err
	}
	pattern := make([]byte, 4096)
	for i := range pattern {
		pattern[i] = byte(i*31 + len(id))
	}
	_, err = f.WriteAt(pattern, 1<<20+0x1000)
	return err
}
