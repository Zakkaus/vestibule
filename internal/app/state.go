package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/store"
)

func loadRuntimeState(configPath, stateDirectory string) (*config.Config, *store.Settings, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, nil, fmt.Errorf("config: %w", err)
	}
	settingsPath := ""
	if stateDirectory != "" {
		if err := os.MkdirAll(stateDirectory, 0o700); err != nil {
			log.Printf("WARNING: cannot create STATE_DIRECTORY %q (%v) — persistence will not work", stateDirectory, err)
		}
		store.ReclaimTemps(stateDirectory)
		settingsPath = filepath.Join(stateDirectory, "settings.json")
	}
	baseline, err := store.LoadBaseline(configPath, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("settings baseline: %w", err)
	}
	runtimeSettings, err := store.NewSettings(settingsPath, baseline)
	if err != nil {
		return nil, nil, fmt.Errorf("settings: %w", err)
	}
	return cfg, runtimeSettings, nil
}
