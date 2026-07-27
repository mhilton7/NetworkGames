package nbd

import "testing"

func FuzzInfoNameParser(f *testing.F) {
	f.Add([]byte{0, 0, 0, 3, 'a', 'l', 'l', 0, 0})
	f.Add([]byte{})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff})
	f.Fuzz(func(t *testing.T, data []byte) {
		name, ok := parseInfoName(data)
		if ok && len(name) > len(data)-6 {
			t.Fatalf("parser returned an out-of-bounds name")
		}
	})
}
