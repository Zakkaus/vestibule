package app

import (
	"context"

	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

// capabilitySettings centralizes post-commit effects for every app-owned settings writer.
// The embedded store preserves the complete settings read and registration surface while Update
// remains the one place where capability changes can trigger runtime effects.
type capabilitySettings struct {
	*settings.Store
	onCapabilityChange func(groupID int64, before, after settings.GroupView)
}

func newCapabilitySettings(store *settings.Store, onChange func(groupID int64, before, after settings.GroupView)) *capabilitySettings {
	return &capabilitySettings{Store: store, onCapabilityChange: onChange}
}
func newRuntimeCapabilitySettings(
	ctx context.Context,
	runtime *services,
	bot *telego.Bot,
	lookups *lookup.Service,
) *capabilitySettings {
	return newCapabilitySettings(runtime.settings, func(groupID int64, before, after settings.GroupView) {
		if runtime.updates != nil {
			runtime.updates.RefreshGroupCommands(context.Background(), bot, groupID)
		}
		if !before.GentooLookupsEnabled().Value && after.GentooLookupsEnabled().Value {
			lookups.DemandWarm(ctx)
		}
	})
}

func (s *capabilitySettings) Update(groupID int64, expectedRevision uint64, next settings.GroupOverrides) (settings.CommitResult, error) {
	before, existed := s.Store.Settings(groupID)
	result, err := s.Store.Update(groupID, expectedRevision, next)
	if err != nil || !existed || s.onCapabilityChange == nil {
		return result, err
	}
	after, exists := s.Store.Settings(groupID)
	if !exists || before.GentooLookupsEnabled().Value == after.GentooLookupsEnabled().Value &&
		before.LinuxLookupsEnabled().Value == after.LinuxLookupsEnabled().Value {
		return result, err
	}
	s.onCapabilityChange(groupID, before, after)
	return result, nil
}
