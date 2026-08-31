package app

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/verification"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const telegramUpdateRetention = 24 * time.Hour

type retentionOutageObserver struct {
	heartbeatPath string
	alert         func(time.Duration)

	mu       sync.Mutex
	reported bool
}

func (o *retentionOutageObserver) observe(now time.Time) {
	outage, ok := heartbeatOutage(o.heartbeatPath, now)
	if !ok {
		return
	}
	o.mu.Lock()
	if outage <= telegramUpdateRetention {
		o.reported = false
		o.mu.Unlock()
		return
	}
	if o.reported {
		o.mu.Unlock()
		return
	}
	o.reported = true
	o.mu.Unlock()
	if o.alert != nil {
		o.alert(outage)
	}
}

func heartbeatOutage(path string, now time.Time) (time.Duration, bool) {
	if path == "" {
		return 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var heartbeat struct {
		LastOnline int64 `json:"last_online"`
	}
	if json.Unmarshal(data, &heartbeat) != nil || heartbeat.LastOnline <= 0 {
		return 0, false
	}
	lastOnline := time.Unix(heartbeat.LastOnline, 0)
	if lastOnline.After(now) {
		return 0, false
	}
	return now.Sub(lastOnline), true
}

type outageAwareBot struct {
	*telego.Bot
	observer *retentionOutageObserver
}

var _ verification.LiveProbe = (*outageAwareBot)(nil)

func (b *outageAwareBot) Unwrap() *telego.Bot { return b.Bot }

func (b *outageAwareBot) GetMe(ctx context.Context) (*telego.User, error) {
	me, err := b.Bot.GetMe(ctx)
	if err == nil {
		b.observer.observe(time.Now())
	}
	return me, err
}

func (b *outageAwareBot) Probe(ctx context.Context) error {
	_, err := b.GetMe(ctx)
	return err
}

func alertRetentionOutage(
	ctx context.Context,
	bot *telego.Bot,
	cfg *config.Config,
	groupIDs []int64,
	outage time.Duration,
) {
	log.Printf("recovery: Telegram outage exceeded update retention (~%s); alerting group administrators", outage.Round(time.Hour))
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, groupID := range groupIDs {
		target := groupID
		if cfg.AdminLogChatID != 0 {
			target = cfg.AdminLogChatID
		}
		language := i18n.FromStored(cfg.LangForGroup(groupID))
		text := i18n.Messages.Verification.Admin.OutageBacklog.Render(language, groupID)
		if _, err := bot.SendMessage(sendCtx, tu.Message(tu.ID(target), text)); err != nil && ctx.Err() == nil {
			log.Printf("recovery: retention alert for group %d failed: %v", groupID, err)
		}
	}
}

func alertPersistenceProblem(ctx context.Context, bot *telego.Bot, cfg *config.Config, settings *store.Settings) {
	status := settings.Persistence()
	if status.LastError == nil {
		return
	}
	log.Printf("WARNING: runtime settings persistence unavailable: %v", status.LastError)
	targets := []int64{cfg.AdminLogChatID}
	if cfg.AdminLogChatID == 0 {
		targets = settings.GroupIDs()
	}
	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, target := range targets {
		if target == 0 {
			continue
		}
		language := i18n.FromStored(cfg.LangForGroup(target))
		text := i18n.Messages.Verification.Admin.SettingsDegraded.Render(language, status.LastError.Error())
		if _, err := bot.SendMessage(sendCtx, tu.Message(tu.ID(target), text)); err != nil && ctx.Err() == nil {
			log.Printf("settings-degraded alert to %d failed: %v", target, err)
		}
	}
}
