package app

import (
	"context"
	"crypto/rand"
	"log"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/feed"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
)

func newOutageAwareBot(
	ctx context.Context,
	bot *telego.Bot,
	settings *settings.Store,
	stateStore verification.Store,
	health *status.Health,
) *outageAwareBot {
	observer := &retentionOutageObserver{
		loadHeartbeat: func() (verification.HeartbeatRecord, error) {
			if stateStore == nil {
				return verification.HeartbeatRecord{}, nil
			}
			return stateStore.LoadHeartbeat("")
		},
	}
	observer.alert = func(outageDuration time.Duration) {
		alertRetentionOutage(ctx, bot, settings, outageDuration)
	}
	return &outageAwareBot{Bot: bot, observer: observer, health: health}
}

func logRuntimeOptions(options Options) {
	if apiURL := strings.TrimSpace(options.TelegramAPIURL); apiURL != "" {
		log.Printf("using Bot API server %s", apiURL)
	}
	if options.GitHubToken != "" {
		log.Printf("GITHUB_TOKEN set — GitHub API rate limit raised (~5000/h)")
	}
	if options.StateDirectory == "" {
		if strings.TrimSpace(options.DatabaseURI) == "" {
			log.Printf("WARNING: STATE_DIRECTORY is unset — persistence is DISABLED: settings changes are runtime-only, and pending verifications, warn counts, and feed cursors will NOT survive a restart (set StateDirectory= in the systemd unit)")
		} else {
			log.Printf("WARNING: STATE_DIRECTORY is unset — settings changes and feed cursors will NOT survive a restart; pending verifications and warn counts use the configured database")
		}
	}
}

func logPrivacyMode(me *telego.User) {
	if !me.CanReadAllGroupMessages {
		log.Printf("NOTE: privacy mode is enabled for this bot, so it does not receive posts sent as a channel; the channel-sender ban (/bc) cannot act until privacy mode is turned off in @BotFather")
	}
}

func startFeeds(ctx context.Context, bot *telego.Bot, stateDirectory string, settingsStore *settings.Store) <-chan struct{} {
	done := make(chan struct{})
	service := feed.New(bot, settingsStore.CurrentFeeds, stateDirectory)
	go func() {
		defer close(done)
		service.Run(ctx)
	}()
	return done
}

func startHeartbeat(ctx context.Context, verification *verification.Service, bot *outageAwareBot) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		verification.RunHeartbeat(ctx, bot)
	}()
	return done
}

func startExpiryScanner(ctx context.Context, verification *verification.Service) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		verification.RunExpiryScanner(ctx)
	}()
	return done
}

func startPendingActions(ctx context.Context, runtime *services) <-chan struct{} {
	done := make(chan struct{})
	releases := importedHoldExecutor{
		store: database.NewVerificationStore(runtime.database), gateway: runtime.verificationGateway,
		owner: "import-unrestrict-" + rand.Text(), now: time.Now,
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			releases.runOnce(ctx)
			runtime.verification.RunPendingActionsOnce(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return done
}

func startDaily(ctx context.Context, daily *status.DailyService) <-chan struct{} {
	if daily == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		daily.Run(ctx)
	}()
	return done
}
