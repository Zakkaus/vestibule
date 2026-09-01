package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/store"
)

func loadRuntimeState(configPath, stateDirectory string, repositories ...settings.Repository) (*settings.Config, *settings.Store, error) {
	cfg, err := settings.LoadConfig(configPath)
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
	baseline, err := settings.LoadBaseline(configPath, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("settings baseline: %w", err)
	}
	if len(repositories) > 1 {
		return nil, nil, fmt.Errorf("settings: at most one repository is supported")
	}
	var repository settings.Repository
	if len(repositories) == 1 {
		repository = repositories[0]
	}
	runtimeSettings, err := settings.NewStore(settingsPath, baseline, repository)
	if err != nil {
		return nil, nil, fmt.Errorf("settings: %w", err)
	}
	return cfg, runtimeSettings, nil
}
