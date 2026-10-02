package app

import (
	"context"
	"os"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func configureWebVerification(runtime *services, consoleURL string) {
	siteKey := strings.TrimSpace(os.Getenv("TURNSTILE_SITE_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("TURNSTILE_SECRET_KEY"))
	runtime.consoleURL, runtime.turnstileSiteKey = consoleURL, siteKey
	available := siteKey != "" && secretKey != ""
	if available {
		client, err := verification.NewTurnstileClient(secretKey, consoleURL, nil)
		if err == nil {
			runtime.turnstileVerifier = client
		} else {
			available = false
		}
	}
	runtime.settings.SetWebCapabilities(settings.WebCapabilities{ConsoleURL: consoleURL, TurnstileAvailable: available})
}

func (runtime *services) webOperatorAlert(ctx context.Context, groupID int64) {
	group, exists := runtime.settings.Settings(groupID)
	if !exists || runtime.verificationGateway == nil {
		return
	}
	target := group.AdminLogChatID().Value
	if target == 0 {
		target = groupID
	}
	runtime.verificationGateway.Alert(ctx, target, i18n.Messages.Verification.Web.OperatorAlert.Render(i18n.FromStored(group.Lang().Value), groupID))
}

func wireWebVerification(runtime *services, service *verification.Service, gateway *telegram.VerificationGateway) {
	service.SetTurnstileVerifier(runtime.turnstileVerifier)
	gateway.SetWebVerification(service, runtime.consoleURL)
}
