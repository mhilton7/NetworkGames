// Command gamecube-volume provides host-side scan, cache-build and validation
// operations without attaching a gadget or touching source game files.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"wiibridge/server/host-daemon/gamecube"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: gamecube-volume scan|build|validate [options]")
	}
	switch args[0] {
	case "scan":
		flags := flag.NewFlagSet("scan", flag.ContinueOnError)
		library := flags.String("library", "", "read-only GameCube library")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *library == "" {
			return errors.New("-library is required")
		}
		result, err := gamecube.Scan(*library)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "build":
		flags := flag.NewFlagSet("build", flag.ContinueOnError)
		library := flags.String("library", "", "read-only GameCube library")
		cache := flags.String("cache", "", "persistent GameCube cache root")
		gameID := flags.String("id", "", "six-character GameCube game ID")
		revision := flags.Uint("revision", 0, "disc revision")
		mode := flags.String("memory-card", "physical", "physical or emulated")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *library == "" || *cache == "" || *gameID == "" || *revision > 255 {
			return errors.New("-library, -cache, valid -id and revision 0..255 are required")
		}
		result, err := gamecube.Scan(*library)
		if err != nil {
			return err
		}
		var selected *gamecube.Game
		for index := range result.Games {
			if result.Games[index].ID == *gameID && result.Games[index].Revision == byte(*revision) {
				selected = &result.Games[index]
				break
			}
		}
		if selected == nil {
			return errors.New("validated GameCube game was not found")
		}
		manifest, err := gamecube.BuildVolume(context.Background(), *cache, *selected,
			gamecube.MemoryCardMode(*mode))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(manifest)
	case "validate":
		flags := flag.NewFlagSet("validate", flag.ContinueOnError)
		manifestPath := flags.String("manifest", "", "completed cache manifest")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *manifestPath == "" {
			return errors.New("-manifest is required")
		}
		manifest, err := gamecube.LoadAndValidateVolume(*manifestPath)
		if err != nil {
			return err
		}
		validation, err := gamecube.ValidateVolume(manifest.ImagePath, manifest)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(validation)
	default:
		return errors.New("unknown command; use scan, build, or validate")
	}
}
