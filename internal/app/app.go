// Package app composes process services and owns their lifecycle.
package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/panel"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// Options contains process inputs read by cmd/bot.
type Options struct {
	ConfigPath     string
	Token          string
	StateDirectory string
	TelegramAPIURL string
	GitHubToken    string
	NotifySocket   string
	Version        string
}

type services struct {
	cfg                 *config.Config
	settings            *store.Settings
	bot                 *telego.Bot
	heartbeatBot        *outageAwareBot
	lookups             *lookup.Service
	verification        *verification.Service
	verificationGateway *telegram.VerificationGateway
	moderation          *moderate.Service
	updates             *telegram.Updates
	registration        *telegram.Registration
	identity            verification.Identity
}

// Run assembles the service graph, starts polling, and drains it on cancellation.
func Run(ctx context.Context, options Options) error {
	if strings.TrimSpace(options.Token) == "" {
		return fmt.Errorf("BOT_TOKEN environment variable is required")
	}
	notifier, err := newSystemdNotifier(options.NotifySocket)
	if err != nil {
		return fmt.Errorf("connect systemd notifier: %w", err)
	}
	defer notifier.close()
	progress := make(chan struct{}, 1)
	startupComplete := make(chan struct{})
	notifierDone := make(chan error, 1)
	go func() { notifierDone <- runSystemdLifecycle(ctx, notifier, startupComplete, progress) }()

	runtime, err := newServices(ctx, options, progress)
	if err != nil {
		return err
	}
	polling, err := startPolling(ctx, runtime)
	if err != nil {
		return fmt.Errorf("start long polling: %w", err)
	}
	feedDone := startFeeds(ctx, runtime.cfg, runtime.bot, options.StateDirectory)
	heartbeatDone := startHeartbeat(ctx, runtime.verification, runtime.heartbeatBot)
	go runtime.lookups.Warm(ctx)
	log.Printf("verify bot @%s (%s) started — groups=%d", runtime.identity.Username, options.Version, len(runtime.settings.GroupIDs()))
	close(startupComplete)
	return runRuntimeLifecycle(ctx, runtimeLifecycle{
		handlerDone:       polling.Done(),
		stopHandlers:      polling.Stop,
		waitRegistration:  runtime.registration.Wait,
		heartbeatDone:     heartbeatDone,
		flushVerification: runtime.verification.Shutdown,
		feedDone:          feedDone,
		notifierDone:      notifierDone,
		shutdownDeadline:  shutdownDeadline,
	})
}

func newServices(ctx context.Context, options Options, progress chan<- struct{}) (*services, error) {
	cfg, settings, err := loadRuntimeState(options.ConfigPath, options.StateDirectory)
	if err != nil {
		return nil, err
	}
	bot, err := newBot(options, progress)
	if err != nil {
		return nil, err
	}
	connector := telegram.NewConnector(bot)
	lookups := lookup.New(settings, connector, cfg, options.GitHubToken)
	logRuntimeOptions(options)
	alertPersistenceProblem(ctx, bot, cfg, settings)
	heartbeatBot := newOutageAwareBot(ctx, bot, cfg, settings, options.StateDirectory)
	// Uptime counts from before the GetMe round trip, as it did previously.
	// Measuring it afterwards silently shortens every uptime an operator reads
	// by however long that call took.
	startedAt := time.Now()
	me, err := heartbeatBot.GetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("GetMe failed (required for the verification deep link): %w", err)
	}
	logPrivacyMode(me)
	identity := verification.Identity{ID: me.ID, Username: me.Username}
	verificationGateway := telegram.NewVerificationGateway(connector)
	verificationStore := database.NewVerificationJSONStore()
	verification := verification.New(settings, verificationGateway, verificationStore, cfg, &i18n.Messages, heartbeatBot, identity, options.StateDirectory)
	moderation := moderate.New(settings, connector, cfg, options.StateDirectory)
	administration := panel.New(
		settings, connector, cfg, &i18n.Messages,
		verification, moderation, lookups, options.Version, startedAt,
	)
	updates := telegram.NewUpdates(cfg, settings, connector, telegramHandlers(verification, verificationGateway, administration, moderation, lookups))
	registration := newRegistration(ctx, bot, cfg, settings, identity, moderation, verification, updates)
	return &services{
		cfg: cfg, settings: settings, bot: bot, heartbeatBot: heartbeatBot,
		lookups: lookups, verification: verification, verificationGateway: verificationGateway, moderation: moderation,
		updates: updates, registration: registration, identity: identity,
	}, nil
}

func newBot(options Options, progress chan<- struct{}) (*telego.Bot, error) {
	botOptions := []telego.BotOption{telegram.WithPollingProgress(progress)}
	if apiURL := strings.TrimSpace(options.TelegramAPIURL); apiURL != "" {
		botOptions = append(botOptions, telego.WithAPIServer(apiURL))
	}
	bot, err := telego.NewBot(options.Token, botOptions...)
	if err != nil {
		return nil, fmt.Errorf("create bot: %w", err)
	}
	return bot, nil
}

func newRegistration(
	ctx context.Context,
	bot *telego.Bot,
	cfg *config.Config,
	settings *store.Settings,
	identity verification.Identity,
	moderation *moderate.Service,
	verification *verification.Service,
	updates *telegram.Updates,
) *telegram.Registration {
	return telegram.NewRegistration(
		ctx, bot, settings, cfg, identity.Username, identity.ID,
		func(checkCtx context.Context) { updates.SetupCommands(checkCtx, bot) },
		func(checkCtx context.Context, groupID int64) {
			moderation.LogGroupSetup(checkCtx, bot, identity.ID, groupID)
			updates.SetupCommands(checkCtx, bot)
		},
		func(checkCtx context.Context, groupID int64) {
			moderation.LogGroupSetup(checkCtx, bot, identity.ID, groupID)
		},
		verification.RemoveGroup,
	)
}

func startPolling(ctx context.Context, runtime *services) (*telegram.Polling, error) {
	if err := runtime.registration.EnsureOwnerClaim(); err != nil {
		log.Printf("WARNING: owner claim is unavailable until durable settings storage is restored: %v", err)
	}
	runtime.updates.SetupCommands(ctx, runtime.bot)
	return telegram.StartPolling(ctx, runtime.bot, func(handler *th.BotHandler) {
		runtime.registration.Register(handler)
		runtime.updates.Register(handler)
	})
}
