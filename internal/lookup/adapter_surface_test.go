package lookup_test

import (
	"context"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func newLookupTestService(store *settings.Store, connector *telegram.Connector, cfg *settings.Config, token string) *telegram.LookupHandlers {
	return telegram.NewLookupHandlers(lookup.New(store, cfg, token), connector)
}

func gentooArmMessageWith(ctx context.Context, l i18n.Lang, name string, search func(context.Context, string) ([]string, bool), status func(context.Context, string) (string, string, bool)) (string, string) {
	result := lookup.TestGentooArmStatusWith(ctx, name, search, status)
	return tgfmt.RenderArmSupport(l, result), result.URL
}
