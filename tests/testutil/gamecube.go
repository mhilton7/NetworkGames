package testutil

import (
	"encoding/binary"
	"errors"
	"os"
)

const (
	GameCubeMagic     = uint32(0xc2339f3d)
	GameCubeFSTOffset = int64(0x1000)
)

// SyntheticGameCubeISO writes a minimal, non-copyrighted disc-image fixture.
// It contains a valid GameCube header and an empty, structurally valid FST.
func SyntheticGameCubeISO(path, id, title string, disc, revision byte, size int64) error {
	if len(id) != 6 {
		return errors.New("GameCube ID must contain six bytes")
	}
	if size < GameCubeFSTOffset+12 {
		size = 2 << 20
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 0x440)
	copy(header[0:6], id)
	header[6] = disc
	header[7] = revision
	binary.BigEndian.PutUint32(header[0x18:0x1c], GameCubeMagic)
	copy(header[0x20:0x60], title)
	binary.BigEndian.PutUint32(header[0x424:0x428], 0x2000)
	binary.BigEndian.PutUint32(header[0x428:0x42c], uint32(GameCubeFSTOffset))
	binary.BigEndian.PutUint32(header[0x42c:0x430], 12)
	binary.BigEndian.PutUint32(header[0x430:0x434], 12)
	if _, err = f.WriteAt(header, 0); err != nil {
		return err
	}
	fst := make([]byte, 12)
	fst[0] = 1 // root directory
	binary.BigEndian.PutUint32(fst[8:12], 1)
	if _, err = f.WriteAt(fst, GameCubeFSTOffset); err != nil {
		return err
	}
	return f.Truncate(size)
}
