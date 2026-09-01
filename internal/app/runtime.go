package app

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/feed"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
)

func newOutageAwareBot(
	ctx context.Context,
	bot *telego.Bot,
	cfg *settings.Config,
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
		alertRetentionOutage(ctx, bot, cfg, settings.ChatIDs(), outageDuration)
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

func startFeeds(ctx context.Context, cfg *settings.Config, bot *telego.Bot, stateDirectory string) <-chan struct{} {
	var feeds []*settings.FeedConfig
	for i := range cfg.Feeds {
		if cfg.Feeds[i].ChatID != 0 {
			feeds = append(feeds, &cfg.Feeds[i])
		} else {
			log.Printf("WARNING: a feed entry has chat_id=0 (missing/invalid) — it is disabled; set its chat_id to the target channel")
		}
	}
	if len(feeds) == 0 {
		return nil
	}
	done := make(chan struct{})
	service := feed.New(bot, feeds, stateDirectory)
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

func startPendingActions(ctx context.Context, verification *verification.Service) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		verification.RunPendingActions(ctx)
	}()
	return done
}
