package gamecube

import "testing"

func TestSettingsRoundTripAndDefaults(t *testing.T) {
	root := t.TempDir()
	settings, err := LoadSettings(root, "GSET01", 2)
	if err != nil {
		t.Fatal(err)
	}
	if settings.MemoryCard != MemoryCardPhysical || settings.VideoMode != "auto" ||
		!settings.ReturnToLoader {
		t.Fatalf("unsafe defaults: %#v", settings)
	}
	settings.MemoryCard = MemoryCardEmulated
	settings.MemoryCardSize = 1
	settings.NativeControl = true
	if err = SaveSettings(root, settings); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSettings(root, settings.GameID, settings.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != settings {
		t.Fatalf("settings mismatch: got %#v want %#v", loaded, settings)
	}
}

func TestSettingsRejectUnsupportedNintendontValues(t *testing.T) {
	settings := DefaultSettings("GSET01", 0)
	settings.MemoryCardSize = 6
	if err := settings.Validate(); err == nil {
		t.Fatal("unsupported memory-card size accepted")
	}
	settings = DefaultSettings("GSET01", 0)
	settings.VideoMode = "forced-random"
	if err := settings.Validate(); err == nil {
		t.Fatal("unsupported video mode accepted")
	}
}
