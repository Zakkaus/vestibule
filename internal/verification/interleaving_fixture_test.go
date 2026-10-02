package verification_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/settings"
	v "github.com/Zakkaus/vestibule/internal/verification"
)

var chaosGroups = [...]int64{-1009000000911, -1009000000912}
var chaosUsers = [...]int64{911, 912}

type chaosPair struct{ group, user int64 }
type chaosFinding struct{ invariant, detail string }
type chaosFixture struct {
	t           *testing.T
	db          *database.Database
	store       *database.VerificationStore
	groups      *settings.Store
	cfg         *settings.Config
	service     *v.Service
	bot         *chaosGateway
	now         time.Time
	directory   string
	canceled    map[string]bool
	noFault     map[string]string
	findings    []chaosFinding
	cancelKind  map[string]string
	witnesses   map[string]string
	currentStep string
	lastFault   string
	interrupt   string
	order       map[string]int
}

func newChaosFixture(t *testing.T) *chaosFixture {
	t.Helper()
	f := &chaosFixture{t: t, directory: t.TempDir(), now: time.Unix(1_900_000_000, 0),
		canceled: make(map[string]bool), noFault: make(map[string]string)}
	f.cancelKind = make(map[string]string)
	f.witnesses = make(map[string]string)
	f.order = make(map[string]int)
	var gateway *logicGateway
	f.db, gateway, _ = logicFixture(t, settings.DeliveryBoth)
	var err error
	f.store = database.NewVerificationStore(f.db)
	f.cfg = &settings.Config{VerifyMode: settings.ModeQuiz, DeliveryMode: settings.DeliveryBoth,
		TimeoutSeconds: 30, MuteSeconds: 30, VerifyMaxFails: -1, VerifyRetrySeconds: 1,
		Questions: []settings.Question{{Q: "Choose yes", Options: []string{"yes", "no"}, Answer: 0}}}
	baseline, err := settings.LoadBaseline("", f.cfg)
	chaosRequire(t, err)
	f.groups, err = settings.NewStore(filepath.Join(f.directory, "settings.json"), baseline, nil, nil)
	chaosRequire(t, err)
	registration := f.groups.Registrations()
	registration.OwnerID = 9
	for _, gid := range chaosGroups {
		registration.RegisteredGroups = append(registration.RegisteredGroups,
			settings.RegisteredGroup{ID: gid, RegisteredBy: 9})
	}
	_, err = f.groups.CommitRegistrations(registration.Revision, registration)
	chaosRequire(t, err)
	for _, gid := range chaosGroups {
		view, _ := f.groups.Settings(gid)
		_, err = f.groups.Update(gid, view.Revision(), settings.GroupOverrides{
			VerifyMode: &f.cfg.VerifyMode, DeliveryMode: &f.cfg.DeliveryMode,
			TimeoutSeconds: &f.cfg.TimeoutSeconds, MuteSeconds: &f.cfg.MuteSeconds,
			VerifyMaxFails: &f.cfg.VerifyMaxFails, VerifyRetrySeconds: &f.cfg.VerifyRetrySeconds,
			Questions: &f.cfg.Questions,
		})
		chaosRequire(t, err)
	}
	f.bot = &chaosGateway{Gateway: gateway, fixture: f,
		members: make(map[chaosPair]chaosMember), faults: make(map[string]string),
		questions: make(map[[2]int64]bool), admitted: make(map[chaosPair]time.Time)}
	chaosRequire(t, f.store.SaveHeartbeat("", v.HeartbeatRecord{LastOnline: f.now.Unix()}))
	f.restart()
	return f
}

func chaosRequire(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func (f *chaosFixture) pending() []v.PendingRecord {
	records, err := f.store.LoadPending("")
	chaosRequire(f.t, err)
	return records
}

func chaosID(r v.PendingRecord) string {
	return fmt.Sprintf("%d:%d:%s", r.GroupID, r.UserID, r.Nonce)
}

func (f *chaosFixture) restart() {
	heartbeat, err := f.store.LoadHeartbeat("")
	chaosRequire(f.t, err)
	for _, record := range f.pending() {
		switch {
		case heartbeat.LastOnline != 0 && f.now.Unix()-heartbeat.LastOnline > 90:
			f.noFault[chaosID(record)] = "recovered"
		case record.Deadline <= f.now.Unix():
			f.noFault[chaosID(record)] = "restart-lapsed"
		}
	}
	f.service = nil
	f.service, err = v.InterleavingService(f.groups, f.bot,
		&chaosClaimStore{Store: f.store, fixture: f}, f.cfg, f.directory,
		func() time.Time { return f.now })
	chaosRequire(f.t, err)
}

func (f *chaosFixture) step(text string) {
	parts := strings.Split(text, "/")
	if parts[0] == "interrupt" {
		f.interrupt = parts[1]
		return
	}
	if parts[0] == "fail" {
		f.bot.faults[parts[1]] = parts[2]
		return
	}
	if parts[0] == "advance" {
		seconds, err := strconv.ParseInt(parts[1], 10, 64)
		chaosRequire(f.t, err)
		f.now = f.now.Add(time.Duration(seconds) * time.Second)
		return
	}
	if parts[0] == "restart" {
		f.restart()
		return
	}
	if parts[0] == "due" {
		f.drain()
		return
	}
	group, user := 0, 0
	if len(parts) == 3 {
		var err error
		group, err = strconv.Atoi(parts[1])
		chaosRequire(f.t, err)
		user, err = strconv.Atoi(parts[2])
		chaosRequire(f.t, err)
	}
	pair := chaosPair{chaosGroups[group], chaosUsers[user]}
	f.targetStep(parts[0], pair)
}

func (f *chaosFixture) targetStep(op string, pair chaosPair) {
	ctx := v.NewHandlerContext(context.Background(), f.bot)
	switch op {
	case "join":
		chaosRequire(f.t, f.service.OnJoinRequest(ctx, v.Update{ChatJoinRequest: &v.ChatJoinRequest{
			Chat: v.Chat{ID: pair.group, Type: v.ChatTypeSupergroup}, From: chaosUser(pair.user)}}))
	case "member":
		member := f.bot.members[pair]
		if !member.present {
			f.bot.admitted[pair] = f.now
		}
		member.present = true
		f.bot.members[pair] = member
		chaosRequire(f.t, f.service.OnMemberJoined(ctx, v.Update{ChatMember: &v.ChatMemberUpdated{
			Chat: v.Chat{ID: pair.group, Type: v.ChatTypeSupergroup}, From: v.User{ID: 999},
			OldChatMember: &v.ChatMemberLeft{Status: v.MemberStatusLeft},
			NewChatMember: &v.ChatMemberMember{Status: v.MemberStatusMember, User: chaosUser(pair.user)}}}))
	case "right", "wrong", "console-approve", "console-reject":
		f.answer(op, pair)
	case "timeout":
		for _, record := range f.pending() {
			if record.GroupID == pair.group && record.UserID == pair.user && record.Deadline > f.now.Unix() {
				f.now = time.Unix(record.Deadline, 0)
			}
		}
		f.service.InterleavingOnline()
		f.service.ScanExpired(context.Background())
	case "stop", "remove":
		f.cancel(op, pair.group)
	default:
		f.t.Fatalf("unknown interleaving step %q", op)
	}
}

func chaosUser(id int64) v.User { return v.User{ID: id, FirstName: "Applicant", LanguageCode: "en"} }

func (f *chaosFixture) answer(op string, pair chaosPair) {
	for _, record := range f.pending() {
		if record.GroupID != pair.group || record.UserID != pair.user {
			continue
		}
		delete(f.noFault, chaosID(record))
		if strings.HasPrefix(op, "console-") {
			target := v.ChallengeApproved
			if op == "console-reject" {
				target = v.ChallengeDeclined
			}
			_, err := f.service.SettleConsole(context.Background(), v.ConsoleSettlement{
				ID: chaosID(record), GroupID: pair.group, ActorID: 9, Expected: v.ChallengePending, Target: target})
			chaosRequire(f.t, err)
		} else {
			choice := record.CorrectIdx
			if op == "wrong" {
				choice = (choice + 1) % len(record.QOpts)
			}
			data := fmt.Sprintf("%s%s:%d", v.AnswerCallbackPrefix, chaosID(record), choice)
			chaosRequire(f.t, f.service.OnAnswer(v.NewHandlerContext(context.Background(), f.bot), v.Update{
				CallbackQuery: &v.CallbackQuery{ID: "chaos-answer", From: chaosUser(pair.user), Data: data}}))
		}
		return
	}
}

func (f *chaosFixture) cancel(op string, group int64) {
	rows, err := f.db.Query(context.Background(), `SELECT id FROM challenge WHERE chat_id=$1 AND
		(state='pending' OR EXISTS (SELECT 1 FROM pending_action WHERE challenge_id=challenge.id
		AND state='pending' AND kind IN ('settle_approve','settle_decline','settle_ban')))`, group)
	chaosRequire(f.t, err)
	for rows.Next() {
		var id string
		chaosRequire(f.t, rows.Scan(&id))
		f.canceled[id] = true
		if f.cancelKind[id] == "" {
			f.cancelKind[id] = op
		}
	}
	chaosRequire(f.t, rows.Err())
	chaosRequire(f.t, rows.Close())
	if !f.groups.IsGroup(group) {
		return
	}
	if op == "stop" {
		chaosRequire(f.t, f.service.SetEnabled(group, false))
		return
	}
	registration := f.groups.Registrations()
	registration.RegisteredGroups = slices.DeleteFunc(registration.RegisteredGroups,
		func(g settings.RegisteredGroup) bool { return g.ID == group })
	_, err = f.groups.CommitRegistrations(registration.Revision, registration)
	chaosRequire(f.t, err)
	f.service.RemoveGroup(group)
}

func (f *chaosFixture) actionCount(due bool) int {
	query := "SELECT COUNT(*) FROM pending_action WHERE state='pending'"
	args := []any{}
	if due {
		query += " AND next_try_at <= $1 AND (claim_until IS NULL OR claim_until <= $1)"
		args = append(args, f.now.Unix())
	}
	var count int
	chaosRequire(f.t, f.db.QueryRow(context.Background(), query, args...).Scan(&count))
	return count
}

func (f *chaosFixture) drain() {
	for range 64 {
		if f.actionCount(true) == 0 {
			return
		}
		f.service.RunPendingActionsOnce(context.Background())
	}
	f.t.Fatal("due action pass did not converge")
}
