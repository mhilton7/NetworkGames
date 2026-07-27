// Command gamecube-saves backs up and restores Nintendont memory-card files in
// a detached GameCube volume. It never attaches a gadget or mounts the image.
package main

import (
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
		return errors.New("usage: gamecube-saves backup|restore [options]")
	}
	switch args[0] {
	case "backup":
		flags := flag.NewFlagSet("backup", flag.ContinueOnError)
		manifestPath := flags.String("manifest", "", "detached volume manifest")
		backupRoot := flags.String("backups", "", "durable save backup directory")
		retain := flags.Int("retain", gamecube.DefaultSaveBackupRetention, "versions retained per card")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *manifestPath == "" || *backupRoot == "" {
			return errors.New("-manifest and -backups are required")
		}
		manifest, err := gamecube.LoadAndValidateVolume(*manifestPath)
		if err != nil {
			return err
		}
		backups, err := gamecube.BackupMemoryCards(manifest, *backupRoot, *retain)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(backups)
	case "restore":
		flags := flag.NewFlagSet("restore", flag.ContinueOnError)
		manifestPath := flags.String("manifest", "", "detached volume manifest")
		backupPath := flags.String("backup", "", "validated .raw backup")
		saveName := flags.String("name", "", "Nintendont save filename, such as GAME.raw")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *manifestPath == "" || *backupPath == "" || *saveName == "" {
			return errors.New("-manifest, -backup and -name are required")
		}
		manifest, err := gamecube.LoadAndValidateVolume(*manifestPath)
		if err != nil {
			return err
		}
		return gamecube.RestoreMemoryCard(manifest, *backupPath, *saveName)
	default:
		return errors.New("unknown command; use backup or restore")
	}
}
