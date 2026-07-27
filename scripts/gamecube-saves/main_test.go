package main

import "testing"

func TestSaveCommandsRequireExplicitDetachedVolumeArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"backup"}, {"restore"}, {"unknown"}} {
		if err := run(args); err == nil {
			t.Fatalf("%v unexpectedly succeeded", args)
		}
	}
}
