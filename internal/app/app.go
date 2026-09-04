// Package app composes process services and owns their lifecycle.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/api"
	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/panel"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/status"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	"github.com/mymmrac/telego/telegoapi"
)

// Options contains process inputs read by cmd/bot.
type Options struct {
	ConfigPath     string
	Token          string
	SetupToken     string
	StateDirectory string
	DatabaseType   string
	DatabaseURI    string
	TelegramAPIURL string
	GitHubToken    string
	NotifySocket   string
	ConsoleAddr    string
	ConsoleURL     string
	Version        string
}

type services struct {
	cfg                  *settings.Config
	database             *database.Database
	settings             *settings.Store
	bot                  *telego.Bot
	heartbeatBot         *outageAwareBot
	lookups              *lookup.Service
	modules              *runtimeModules
	verification         *verification.Service
	verificationGateway  verification.Gateway
	moderation           *moderate.Service
	updates              *telegram.Updates
	registration         *telegram.Registration
	consoleAuth          *auth.Manager
	health               *status.Health
	replacement          *status.Replacement
	release              *status.ReleaseChecker
	rollbackObservations *status.RollbackObservations
	version              string
	identity             verification.Identity
}

type activeRuntime struct {
	context       context.Context
	cancel        context.CancelFunc
	runtime       *services
	polling       *pollingLease
	feedDone      <-chan struct{}
	heartbeatDone <-chan struct{}
	expiryDone    <-chan struct{}
	actionDone    <-chan struct{}
}

// Run assembles the service graph, starts polling after a claim, and drains it on cancellation.
func Run(ctx context.Context, options Options) error {
	notifier, err := newSystemdNotifier(options.NotifySocket)
	if err != nil {
		return fmt.Errorf("connect systemd notifier: %w", err)
	}
	defer notifier.close()
	progress := make(chan struct{}, 1)
	startupComplete := make(chan struct{})
	notifierDone := make(chan error, 1)
	go func() { notifierDone <- runSystemdLifecycle(ctx, notifier, startupComplete, progress) }()

	runtime, err := newBaseServices(ctx, options)
	if err != nil {
		return err
	}
	defer closeRuntimeDatabase(runtime)

	claimState, err := openSetupState(options.StateDirectory, options.SetupToken)
	if err != nil {
		return err
	}
	if token := strings.TrimSpace(options.Token); token != "" {
		options.Token = token
	} else {
		options.Token = claimState.BotToken()
	}
	if options.Token != "" {
		return runClaimed(ctx, options, runtime, progress, startupComplete, notifierDone)
	}
	return runUnclaimed(ctx, options, runtime, claimState, progress, startupComplete, notifierDone)
}

func runClaimed(
	ctx context.Context,
	options Options,
	runtime *services,
	progress chan<- struct{},
	startupComplete chan<- struct{},
	notifierDone <-chan error,
) error {
	if err := activateServices(ctx, runtime, options, progress); err != nil {
		return err
	}
	active, err := startActiveRuntime(ctx, runtime)
	if err != nil {
		return err
	}
	defer active.cancel()
	console := api.New(claimedConsoleConfig(runtime))
	runtime.health.SetTelegramReady(true)
	if err := console.Start(consoleAddress(options.ConsoleAddr)); err != nil {
		runtime.health.SetTelegramReady(false)
		stopActiveRuntime(active)
		return fmt.Errorf("start console HTTP: %w", err)
	}
	log.Printf("verify bot @%s (%s) started — groups=%d", runtime.identity.Username, options.Version, len(runtime.settings.ChatIDs()))
	close(startupComplete)
	return runActiveLifecycle(active, console, notifierDone)
}

func runUnclaimed(
	ctx context.Context,
	options Options,
	runtime *services,
	claimState *setupState,
	progress chan<- struct{},
	startupComplete chan<- struct{},
	notifierDone <-chan error,
) error {
	activated := make(chan *activeRuntime, 1)
	coordinator := newSetupCoordinator(ctx, options, runtime, claimState, progress, activated)
	console := api.New(bootstrapConsoleConfig(runtime, coordinator, nil))
	console.ReplaceRoutes(bootstrapConsoleConfig(runtime, coordinator, func() {
		runtime.health.SetTelegramReady(true)
		console.ReplaceRoutes(claimedConsoleConfig(runtime))
	}))
	if err := console.Start(consoleAddress(options.ConsoleAddr)); err != nil {
		return fmt.Errorf("start console HTTP: %w", err)
	}
	log.Printf("instance is unclaimed; waiting for a setup claim")
	close(startupComplete)
	select {
	case active := <-activated:
		defer active.cancel()
		log.Printf("verify bot @%s (%s) started — groups=%d", runtime.identity.Username, options.Version, len(runtime.settings.ChatIDs()))
		return runActiveLifecycle(active, console, notifierDone)
	case <-ctx.Done():
		coordinator.stop()
		select {
		case active := <-activated:
			stopActiveRuntime(active)
		default:
		}
		shutdownBootstrap(console, notifierDone)
		return nil
	}
}

func shutdownBootstrap(console *api.Server, notifierDone <-chan error) {
	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownDeadline)
	defer cancel()
	if err := console.Shutdown(stopCtx); err != nil {
		log.Printf("shutdown: console HTTP did not drain cleanly: %v", err)
	}
	waitForNotifier(stopCtx, notifierDone)
}

func closeRuntimeDatabase(runtime *services) {
	if err := runtime.database.Close(); err != nil {
		log.Printf("database close failed: %v", err)
	}
}

func claimedConsoleConfig(runtime *services) api.Config {
	return api.Config{
		Authenticator:        runtime.consoleAuth,
		Verification:         runtime.verification,
		Settings:             runtime.settings,
		Rules:                database.NewRuleStore(runtime.database),
		ProcessSettings:      runtime.cfg,
		Health:               runtime.health,
		Persistence:          runtime.settings,
		RollbackObservations: runtime.rollbackObservations,
		RollbackRejections:   runtime.verification,
		Replacement:          runtime.replacement,
		Release:              runtime.release,
		Version:              runtime.version,
		ObserveOnly:          runtime.cfg.ObserveOnly,
	}
}

func bootstrapConsoleConfig(runtime *services, setup api.SetupService, setupClaimed func()) api.Config {
	return api.Config{
		Settings:        runtime.settings,
		ProcessSettings: runtime.cfg,
		Health:          runtime.health,
		Persistence:     runtime.settings,
		Setup:           setup,
		Replacement:     runtime.replacement,
		Release:         runtime.release,
		Version:         runtime.version,
		ObserveOnly:     runtime.cfg.ObserveOnly,
		SetupClaimed:    setupClaimed,
	}
}

func startActiveRuntime(parent context.Context, runtime *services) (*activeRuntime, error) {
	runtimeCtx, cancel := context.WithCancel(parent)
	polling, err := newRuntimePollingLease(runtimeCtx, runtime)
	if err != nil {
		cancel()
		return nil, err
	}
	active := &activeRuntime{
		context: runtimeCtx, cancel: cancel, runtime: runtime, polling: polling,
		feedDone:      runtime.modules.Start(runtimeCtx),
		heartbeatDone: startHeartbeat(runtimeCtx, runtime.verification, runtime.heartbeatBot),
		expiryDone:    startExpiryScanner(runtimeCtx, runtime.verification),
		actionDone:    startPendingActions(runtimeCtx, runtime.verification),
	}
	if err := polling.Start(runtimeCtx, runtime); err != nil {
		stopActiveRuntime(active)
		return nil, fmt.Errorf("start long polling: %w", err)
	}
	return active, nil
}

func stopActiveRuntime(active *activeRuntime) {
	active.cancel()
	stopCtx, cancel := context.WithTimeout(context.Background(), shutdownDeadline)
	defer cancel()
	_ = active.polling.Stop(stopCtx)
	active.runtime.verification.Shutdown()
}

func runActiveLifecycle(active *activeRuntime, console *api.Server, notifierDone <-chan error) error {
	return runRuntimeLifecycle(active.context, runtimeLifecycle{
		handlerDone:       active.polling.Done(),
		stopHandlers:      active.polling.Stop,
		waitRegistration:  active.runtime.registration.Wait,
		heartbeatDone:     active.heartbeatDone,
		expiryDone:        active.expiryDone,
		actionDone:        active.actionDone,
		flushVerification: active.runtime.verification.Shutdown,
		feedDone:          active.feedDone,
		notifierDone:      notifierDone,
		stopAdmission:     console.StopAdmission,
		shutdownHTTP:      console.Shutdown,
		shutdownDeadline:  shutdownDeadline,
	})
}

func newServices(ctx context.Context, options Options, progress chan<- struct{}) (*services, error) {
	runtime, err := newBaseServices(ctx, options)
	if err != nil {
		return nil, err
	}
	if err := activateServices(ctx, runtime, options, progress); err != nil {
		_ = runtime.database.Close()
		return nil, err
	}
	return runtime, nil
}

func newBaseServices(ctx context.Context, options Options) (*services, error) {
	db, err := database.Open(ctx, database.Config{
		Type: options.DatabaseType, URI: options.DatabaseURI, StateDirectory: options.StateDirectory,
	})
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	health := status.NewHealth(func(checkCtx context.Context) error {
		return db.RawDB.PingContext(checkCtx)
	})
	cfg, runtimeSettings, err := loadRuntimeState(
		options.ConfigPath,
		options.StateDirectory,
		database.NewSettingsStore(db),
	)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	health.SetConfigReady(true)
	return &services{
		database: db, cfg: cfg, settings: runtimeSettings, health: health,
		replacement:          status.NewReplacement(options.StateDirectory),
		release:              status.NewReleaseChecker(options.Version, options.GitHubToken),
		rollbackObservations: status.NewRollbackObservations(time.Now),
		version:              options.Version,
	}, nil
}

func activateServices(ctx context.Context, runtime *services, options Options, progress chan<- struct{}) error {
	if strings.TrimSpace(options.Token) == "" {
		return fmt.Errorf("a bot token is required to activate the instance")
	}
	bot, err := newBot(options, progress)
	if err != nil {
		return fmt.Errorf("%w: %w", api.ErrSetupTokenRejected, err)
	}
	connector := telegram.NewConnector(bot)
	consoleAuth, consoleHandler, err := newConsoleAuthentication(
		options, runtime.settings, connector, bot, runtime.rollbackObservations,
	)
	if err != nil {
		return err
	}
	lookups := lookup.New(runtime.settings, connector, runtime.cfg, options.GitHubToken)
	logRuntimeOptions(options)
	alertPersistenceProblem(ctx, bot, runtime.cfg, runtime.settings)
	verificationStore := database.NewVerificationStore(runtime.database)
	heartbeatBot := newOutageAwareBot(ctx, bot, runtime.cfg, runtime.settings, verificationStore, runtime.health)
	// Count uptime before GetMe so operator-visible uptime includes its latency.
	startedAt := time.Now()
	me, err := heartbeatBot.GetMe(ctx)
	if err != nil {
		return fmt.Errorf("%w: GetMe failed (required for the verification deep link): %w", claimFailureFor(err), err)
	}
	logPrivacyMode(me)
	identity := verification.Identity{ID: me.ID, Username: me.Username}
	liveVerificationGateway := telegram.NewVerificationGateway(connector)
	verificationGateway, err := verificationGatewayForMode(
		ctx, runtime.cfg, runtime.database, liveVerificationGateway,
	)
	if err != nil {
		return err
	}
	stateNamespace := verificationStateNamespace(options.StateDirectory)
	moderation, err := moderate.New(runtime.settings, connector, runtime.cfg, database.NewWarningStore(runtime.database))
	if err != nil {
		return fmt.Errorf("moderation: %w", err)
	}
	verificationService, err := verification.New(
		runtime.settings, verificationGateway, verificationStore, runtime.cfg, &i18n.Messages,
		heartbeatBot, identity, stateNamespace, runtime.rollbackObservations,
	)
	if err != nil {
		return fmt.Errorf("verification: %w", err)
	}
	administration := panel.New(
		runtime.settings, connector, runtime.cfg, &i18n.Messages,
		verificationService, moderation, lookups, options.Version, startedAt,
	)
	modules, err := newRuntimeModules(runtime.cfg, bot, options.StateDirectory, administration, moderation, lookups)
	if err != nil {
		return err
	}
	administration.SetCommandModules(modules.commands)
	updates := telegram.NewUpdates(runtime.cfg, runtime.settings, connector,
		telegramHandlers(verificationService, verificationGateway, administration, moderation, modules.commands, consoleHandler))
	registration := newRegistration(ctx, bot, runtime.cfg, runtime.settings, identity, moderation, verificationService, updates)
	runtime.bot = bot
	runtime.heartbeatBot = heartbeatBot
	runtime.lookups = lookups
	runtime.modules = modules
	runtime.verification = verificationService
	runtime.verificationGateway = verificationGateway
	runtime.moderation = moderation
	runtime.updates = updates
	runtime.registration = registration
	runtime.consoleAuth = consoleAuth
	runtime.identity = identity
	return nil
}

// claimFailureFor says which half of a failed GetMe the person on the setup page
// can act on. A token Telegram answered and refused has to be pasted again; a
// Telegram that never answered has to be fixed on the machine, and the token is
// not the problem. One message for both sends half of them to the wrong place.
func claimFailureFor(err error) error {
	var apiError *telegoapi.Error
	if errors.As(err, &apiError) && apiError.ErrorCode == http.StatusUnauthorized {
		return api.ErrSetupTokenRejected
	}
	return api.ErrSetupTelegramUnreachable
}

func verificationStateNamespace(stateDirectory string) string {
	if stateDirectory == "" {
		return "database"
	}
	return stateDirectory
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
	cfg *settings.Config,
	settings *settings.Store,
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
