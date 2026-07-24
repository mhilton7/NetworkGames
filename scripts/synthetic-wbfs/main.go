package main

import (
	"fmt"
	"os"

	"networkgames/tests/testutil"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: synthetic-wbfs PATH GAMEID")
		os.Exit(64)
	}
	if err := testutil.SyntheticWBFS(os.Args[1], os.Args[2], "Synthetic fixture", 2<<20); err != nil {
		panic(err)
	}
}
