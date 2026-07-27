package gamecube

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Settings struct {
	GameID         string         `json:"game_id"`
	Revision       byte           `json:"revision"`
	MemoryCard     MemoryCardMode `json:"memory_card_mode"`
	MemoryCardSize byte           `json:"memory_card_size"`
	MultiCard      bool           `json:"multi_card"`
	NativeControl  bool           `json:"native_gamecube_control"`
	VideoMode      string         `json:"video_mode"`
	Progressive    bool           `json:"progressive"`
	PAL50Patch     bool           `json:"pal50_patch"`
	Widescreen     bool           `json:"widescreen_patch"`
	VideoWidth     int            `json:"video_width"`
	VideoOffset    int            `json:"video_offset"`
	Cheats         bool           `json:"cheats"`
	UseIPL         bool           `json:"use_gamecube_ipl"`
	ControllerMode string         `json:"controller_mode"`
	DiscSpeed      string         `json:"disc_speed"`
	ReturnToLoader bool           `json:"return_to_loader"`
}

func DefaultSettings(id string, revision byte) Settings {
	return Settings{
		GameID: id, Revision: revision, MemoryCard: MemoryCardPhysical,
		MemoryCardSize: 2, VideoMode: "auto", ControllerMode: "auto",
		DiscSpeed: "auto", ReturnToLoader: true,
	}
}

func (s Settings) Validate() error {
	if !validID.MatchString(s.GameID) {
		return errors.New("invalid GameCube game ID")
	}
	if s.MemoryCard != MemoryCardPhysical && s.MemoryCard != MemoryCardEmulated {
		return errors.New("memory-card mode must be physical or emulated")
	}
	if s.MemoryCardSize > 5 {
		return errors.New("memory-card size must be 0..5 (59..2043 blocks)")
	}
	switch s.VideoMode {
	case "auto", "disc", "ntsc", "pal50", "pal60", "mpal":
	default:
		return errors.New("unsupported Nintendont video mode")
	}
	if s.VideoWidth < 0 || s.VideoWidth > 2 || s.VideoOffset < -20 || s.VideoOffset > 20 {
		return errors.New("video width or offset is outside the supported UI range")
	}
	switch s.ControllerMode {
	case "auto", "native", "hid":
	default:
		return errors.New("unsupported controller mode")
	}
	switch s.DiscSpeed {
	case "auto", "original":
	default:
		return errors.New("unsupported disc-speed mode")
	}
	return nil
}

func SettingsPath(root, id string, revision byte) string {
	return filepath.Join(root, "settings", fmt.Sprintf("%s-r%d.json", id, revision))
}

func SaveSettings(root string, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	directory := filepath.Join(root, "settings")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	path := SettingsPath(root, settings.GameID, settings.Revision)
	temp, err := os.CreateTemp(directory, ".settings-")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err = temp.Chmod(0o600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func LoadSettings(root, id string, revision byte) (Settings, error) {
	data, err := os.ReadFile(SettingsPath(root, id, revision))
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(id, revision), nil
	}
	if err != nil {
		return Settings{}, err
	}
	var settings Settings
	if err = json.Unmarshal(data, &settings); err != nil {
		return Settings{}, err
	}
	if err = settings.Validate(); err != nil {
		return Settings{}, err
	}
	return settings, nil
}
