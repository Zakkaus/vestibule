package app

import (
	"context"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func newDailyService(
	runtime *services,
	verificationService *verification.Service,
	transport verification.Gateway,
	now func() time.Time,
) *status.DailyService {
	return status.NewDailyService(status.DailyConfig{
		Store:        database.NewDailyStatusStore(runtime.database),
		Health:       runtime.health,
		Observations: runtime.rollbackObservations,
		PendingCount: func(context.Context) (int, error) {
			return verificationService.PendingCount(), nil
		},
		OwnerID: func() int64 {
			return runtime.settings.Registrations().OwnerID
		},
		Send: func(sendCtx context.Context, ownerID int64, text string) error {
			_, sendErr := transport.Send(sendCtx, verification.OutgoingMessage{
				ChatID: ownerID, Text: text,
			})
			return sendErr
		},
		Location: verificationService.StatsLocation(),
		Language: i18n.FromStored(runtime.cfg.Lang),
		Now:      now,
	})
}
