// Package telegram owns Telegram protocol conversion and transport. It routes updates and performs
// Bot API calls; it must not decide verification or moderation outcomes.
package telegram

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
	th "github.com/mymmrac/telego/telegohandler"
)

const (
	pollRetryInterval           = 5 * time.Second
	maxConcurrentUpdateHandlers = 64
	botAPITimeout               = 45 * time.Second
)

var allowedUpdateTypes = [...]string{
	"chat_join_request",
	"chat_member",
	"callback_query",
	"message",
	"my_chat_member",
}

// AllowedUpdateTypes returns the five update kinds required by every long-polling request.
func AllowedUpdateTypes() []string {
	return append([]string(nil), allowedUpdateTypes[:]...)
}

// VerificationHandlers maps Telegram verification updates to the existing verification service.
type VerificationHandlers struct {
	Answer               th.Handler
	AdminAction          th.Handler
	ChannelRecheck       th.Handler
	JoinRequest          th.Handler
	MemberJoined         th.Handler
	KernelAnswer         th.Handler
	KernelAnswerDM       th.Predicate
	AnswerPrefix         string
	AdminPrefix          string
	ChannelRecheckPrefix string
}

// PanelHandlers maps Telegram administration callbacks and input routes to the panel service.
type PanelHandlers struct {
	SettingsCallback th.Handler
	ChatShared       th.Handler
	Input            th.Handler
	ChatSharedDM     th.Predicate
	InputDM          th.Predicate
	SettingsPrefix   string
}

// ModerationHandlers maps non-command moderation updates without making policy decisions.
type ModerationHandlers struct {
	FilterChannelSenders th.Handler
}

// NewModerationHandlers converts sender-chat updates before invoking moderation policy.
func NewModerationHandlers(service *moderate.Service) ModerationHandlers {
	return ModerationHandlers{FilterChannelSenders: channelSenderHandler(service)}
}

func channelSenderHandler(service *moderate.Service) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		message := update.Message
		if message == nil || message.SenderChat == nil {
			return ctx.Next(update)
		}
		if service.FilterChannelSender(ctx.Context(), moderate.ChannelSenderMessage{
			ChatID:           message.Chat.ID,
			MessageID:        message.MessageID,
			SenderChatID:     message.SenderChat.ID,
			SenderChatTitle:  message.SenderChat.Title,
			AutomaticForward: message.IsAutomaticForward,
		}) {
			return nil
		}
		return ctx.Next(update)
	}
}

// NewBlockChannelHandler adapts the /bc command to moderation policy.
func NewBlockChannelHandler(service *moderate.Service) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		message := update.Message
		if message == nil || message.From == nil {
			return nil
		}
		service.BlockChannel(ctx.Context(), moderate.ChannelSenderCommand{
			ChatID:    message.Chat.ID,
			MessageID: message.MessageID,
			CallerID:  message.From.ID,
			Text:      message.Text,
		})
		return nil
	}
}

// HandlerSet is the protocol-facing call surface supplied by app assembly.
type HandlerSet struct {
	Verification VerificationHandlers
	Panel        PanelHandlers
	Moderation   ModerationHandlers
	Commands     CommandModules
	Console      th.Handler
}

type handlerRoute struct {
	name       string
	handler    th.Handler
	predicates []th.Predicate
}

// Updates owns first-match routing, command menus, and bounded direct-message throttling.
type Updates struct {
	cfg      *settings.Config
	settings *settings.Store
	handlers HandlerSet
	dm       *dmHandler
}

// NewUpdates creates the Telegram update router without starting polling.
func NewUpdates(cfg *settings.Config, settings *settings.Store, connector *Connector, handlers HandlerSet) *Updates {
	return &Updates{
		cfg:      cfg,
		settings: settings,
		handlers: handlers,
		dm: &dmHandler{
			cfg:            cfg,
			settings:       settings,
			telegram:       connector,
			commands:       handlers.Commands.MemberCommandNames(),
			last:           make(map[int64]time.Time),
			catalogueReply: isBuiltInPrivateReply(cfg.PrivateReply),
		},
	}
}

// Register installs middleware and handlers in their first-match behavioral order.
func (u *Updates) Register(handler *th.BotHandler) {
	handler.Use(th.PanicRecoveryHandler(func(recovered any) error {
		log.Printf("recovered from handler panic: %v", recovered)
		return nil
	}))
	handler.Use(u.handlers.Moderation.FilterChannelSenders)
	for _, route := range u.handlerRoutes() {
		if route.handler != nil {
			handler.Handle(route.handler, route.predicates...)
		}
	}
}

func (u *Updates) handlerRoutes() []handlerRoute {
	v, p := u.handlers.Verification, u.handlers.Panel
	privateDM := privateNonStart(nil)
	if u.dm != nil {
		privateDM = privateNonStart(u.dm.commands)
	}
	routes := []handlerRoute{
		{name: "verify.answer", handler: v.Answer, predicates: []th.Predicate{th.CallbackDataPrefix(v.AnswerPrefix)}},
		{name: "verify.admin_action", handler: v.AdminAction, predicates: []th.Predicate{th.CallbackDataPrefix(v.AdminPrefix)}},
		{name: "verify.channel_recheck", handler: v.ChannelRecheck, predicates: []th.Predicate{th.CallbackDataPrefix(v.ChannelRecheckPrefix)}},
		{name: "panel.settings_callback", handler: p.SettingsCallback, predicates: []th.Predicate{th.CallbackDataPrefix(p.SettingsPrefix)}},
		{name: "verify.join_request", handler: v.JoinRequest, predicates: []th.Predicate{th.AnyChatJoinRequest()}},
		{name: "verify.member_joined", handler: v.MemberJoined, predicates: []th.Predicate{th.AnyChatMember()}},
		{name: "panel.chat_shared", handler: p.ChatShared, predicates: []th.Predicate{p.ChatSharedDM}},
		{name: "panel.input", handler: p.Input, predicates: []th.Predicate{p.InputDM}},
		{name: "verify.kernel_answer", handler: v.KernelAnswer, predicates: []th.Predicate{v.KernelAnswerDM}},
		{name: "console.open", handler: u.handlers.Console, predicates: []th.Predicate{th.And(th.CommandEqual("console"), privateMessage)}},
		{name: "bot.private_dm", handler: u.dm.onPrivateDM, predicates: []th.Predicate{privateDM}},
	}
	for _, command := range u.handlers.Commands.Routes() {
		routes = append(routes, handlerRoute{
			name: command.Name, handler: command.Handler, predicates: []th.Predicate{th.CommandEqual(command.Command)},
		})
	}
	return routes
}

// Polling owns one running telegohandler and its completion result.
type Polling struct {
	handler *th.BotHandler
	done    <-chan error
}

// Done reports an unexpected handler or update-stream termination.
func (p *Polling) Done() <-chan error { return p.done }

// Stop stops admission and waits for handler shutdown within ctx.
func (p *Polling) Stop(ctx context.Context) error { return p.handler.StopWithContext(ctx) }

// StartPolling registers handlers before opening Telegram long polling.
func StartPolling(ctx context.Context, bot *telego.Bot, register func(*th.BotHandler)) (*Polling, error) {
	handler, done, err := prepareUpdateHandler(ctx, bot, register, func() (<-chan telego.Update, error) {
		return bot.UpdatesViaLongPolling(ctx, &telego.GetUpdatesParams{
			Timeout:        30,
			AllowedUpdates: AllowedUpdateTypes(),
		}, telego.WithLongPollingRetryTimeout(pollRetryInterval))
	})
	if err != nil {
		return nil, err
	}
	return &Polling{handler: handler, done: done}, nil
}

func prepareUpdateHandler(
	ctx context.Context,
	bot *telego.Bot,
	register func(*th.BotHandler),
	startPolling func() (<-chan telego.Update, error),
) (*th.BotHandler, <-chan error, error) {
	handlerUpdates := make(chan telego.Update)
	inFlight := make(chan struct{}, maxConcurrentUpdateHandlers)
	handler, err := th.NewBotHandler(bot, handlerUpdates)
	if err != nil {
		return nil, nil, err
	}
	handler.Use(func(handlerCtx *th.Context, update telego.Update) error {
		defer func() { <-inFlight }()
		return handlerCtx.Next(update)
	})
	register(handler)
	handlerDone := make(chan error, 1)
	go func() { handlerDone <- handler.Start() }()
	for !handler.IsRunning() {
		select {
		case err := <-handlerDone:
			return nil, nil, fmt.Errorf("update handler stopped before polling started: %v", err)
		default:
			runtime.Gosched()
		}
	}
	updates, err := startPolling()
	if err != nil {
		close(handlerUpdates)
		_ = handler.Stop()
		<-handlerDone
		return nil, nil, err
	}
	go forwardUpdates(ctx, updates, handlerUpdates, inFlight)
	return handler, handlerDone, nil
}

func forwardUpdates(ctx context.Context, source <-chan telego.Update, destination chan<- telego.Update, inFlight chan struct{}) {
	defer close(destination)
	draining := false
	for {
		var update telego.Update
		var ok bool
		if draining {
			update, ok = <-source
		} else {
			select {
			case <-ctx.Done():
				draining = true
				continue
			case update, ok = <-source:
			}
		}
		if !ok {
			return
		}
		acquired := false
		if !draining {
			select {
			case <-ctx.Done():
				draining = true
			case inFlight <- struct{}{}:
				acquired = true
			}
		}
		if !acquired {
			inFlight <- struct{}{}
		}
		if !sendUpdate(ctx, destination, update, &draining) {
			destination <- update
		}
	}
}

func sendUpdate(ctx context.Context, destination chan<- telego.Update, update telego.Update, draining *bool) bool {
	if *draining {
		return false
	}
	select {
	case <-ctx.Done():
		*draining = true
		return false
	case destination <- update:
		return true
	}
}

type pollingProgressCaller struct {
	next     ta.Caller
	timeout  time.Duration
	progress func()
}

func (c pollingProgressCaller) Call(ctx context.Context, url string, data *ta.RequestData) (*ta.Response, error) {
	if !strings.HasSuffix(url, "/getUpdates") {
		return c.next.Call(ctx, url, data)
	}
	timeout := c.timeout
	if timeout <= 0 {
		timeout = botAPITimeout
	}
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	response, err := c.next.Call(pollCtx, url, data)
	cancel()
	if c.progress != nil {
		c.progress()
	}
	return response, err
}

// WithPollingProgress bounds a stuck getUpdates call and signals completed attempts.
func WithPollingProgress(progress chan<- struct{}) telego.BotOption {
	return telego.WithAPICaller(pollingProgressCaller{
		next:    ta.HTTPCaller{Client: &http.Client{Timeout: botAPITimeout}},
		timeout: botAPITimeout,
		progress: func() {
			select {
			case progress <- struct{}{}:
			default:
			}
		},
	})
}
