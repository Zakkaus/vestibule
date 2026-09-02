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
		OperatorAllowed: func(telegramID int64) bool { return state.Registrations().OwnerID == telegramID },
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
