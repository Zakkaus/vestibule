package app

import (
	"fmt"
	"strings"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

const defaultConsoleAddress = "127.0.0.1:8080"

func consoleAddress(value string) string {
	if address := strings.TrimSpace(value); address != "" {
		return address
	}
	return defaultConsoleAddress
}

func newConsoleAuthentication(
	options Options,
	state *settings.Store,
	connector *telegram.Connector,
	bot *telego.Bot,
	accessObserver auth.AccessAvailabilityObserver,
) (*auth.Manager, th.Handler, error) {
	manager, err := auth.New(auth.Config{
		BotToken: options.Token, AdminChecker: connector,
		OperatorAllowed: operatorIsOwner(state),
		AccessObserver:  accessObserver,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("console authentication: %w", err)
	}
	handler, err := telegram.NewConsoleLinkHandler(bot, manager, options.ConsoleURL)
	if err != nil {
		return nil, nil, fmt.Errorf("console link: %w", err)
	}
	return manager, handler, nil
}

// operatorIsOwner answers the one question that decides who can hold an operator session.
// It is a named function so a test can ask it directly: written inline, the comparison was
// the only thing between a stranger and the console and nothing could reach it.
//
// An unclaimed instance has no owner, and zero is not a Telegram account, so nobody is the
// owner until the claim records one.
func operatorIsOwner(state *settings.Store) func(int64) bool {
	return func(telegramID int64) bool {
		owner := state.Registrations().OwnerID
		return owner != 0 && owner == telegramID
	}
}
