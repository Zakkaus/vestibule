package app

import (
	"context"
	"fmt"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func verificationGatewayForMode(
	ctx context.Context,
	cfg *settings.Config,
	db *database.Database,
	live verification.Gateway,
) (verification.Gateway, error) {
	if !cfg.ObserveOnly {
		return verification.ApplyObservationMode(live, nil, false), nil
	}
	recorder, err := database.NewObservationStore(ctx, db, time.Now)
	if err != nil {
		return nil, fmt.Errorf("observe-only mode: %w", err)
	}
	return verification.ApplyObservationMode(live, recorder, true), nil
}
