package verification

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestACompletedQuizAcknowledgesWithoutAWarning(t *testing.T) {
	const gid, uid = int64(-100900000101), int64(501)
	key := pkey{gid: gid, uid: uid}
	for _, tc := range []struct {
		name        string
		pending     *pending
		wantApprove int
		wantPending bool
	}{
		{name: "missing challenge", wantPending: false},
		{
			name:        "settled challenge",
			pending:     &pending{nonce: "current", correctIdx: 1, done: true, deadline: time.Now().Add(time.Hour)},
			wantPending: true,
		},
		{
			name:        "live challenge",
			pending:     &pending{nonce: "current", correctIdx: 1, deadline: time.Now().Add(time.Hour)},
			wantApprove: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestService(&settings.Config{GroupIDs: []int64{gid}, VerifyMaxFails: 3})
			if tc.pending != nil {
				v.pend[key] = tc.pending
			}
			bot := newFakeVerifyBot()
			err := v.OnAnswer(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), Update{
				CallbackQuery: &CallbackQuery{
					ID: "completed-quiz", From: User{ID: uid, LanguageCode: "en"},
					Data: AnswerCallbackPrefix + "-100900000101:501:current:1",
				},
			})
			if err != nil {
				t.Fatalf("completed quiz callback returned %v", err)
			}
			if bot.approves != tc.wantApprove || bot.declines != 0 || bot.bans != 0 {
				t.Fatalf("completed quiz actions = approve %d decline %d ban %d, want %d/0/0",
					bot.approves, bot.declines, bot.bans, tc.wantApprove)
			}
			_, pending := v.pend[key]
			if pending != tc.wantPending {
				t.Fatalf("completed quiz pending = %t, want %t", pending, tc.wantPending)
			}
			if len(bot.callbackAnswers) != 1 {
				t.Fatalf("completed quiz acknowledgements = %d, want 1", len(bot.callbackAnswers))
			}
			if tc.wantApprove == 0 {
				answer := bot.callbackAnswers[0]
				want := v.messages.Verification.Result.AlreadyHandled.For(i18n.LangEN)
				if answer.ShowAlert || answer.Text != want {
					t.Fatalf("completed quiz acknowledgement = %#v, want a non-alert already-handled response %q", answer, want)
				}
			}
		})
	}
}

func TestAStaleQuizCallbackNamesTheQuestionThatWasReplaced(t *testing.T) {
	const gid, uid = int64(-100900000102), int64(502)
	for _, tc := range []struct {
		name        string
		nonce       string
		wantApprove int
		wantStale   bool
	}{
		{name: "current question", nonce: "current", wantApprove: 1},
		{name: "replaced question", nonce: "stale", wantStale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestService(&settings.Config{GroupIDs: []int64{gid}, VerifyMaxFails: 3})
			key := pkey{gid: gid, uid: uid}
			v.pend[key] = &pending{
				nonce: "current", correctIdx: 1, lang: i18n.LangEN, deadline: time.Now().Add(time.Hour),
			}
			bot := newFakeVerifyBot()
			err := v.OnAnswer(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), Update{
				CallbackQuery: &CallbackQuery{
					ID: "stale-quiz", From: User{ID: uid, LanguageCode: "en"},
					Data: AnswerCallbackPrefix + "-100900000102:502:" + tc.nonce + ":1",
				},
			})
			if err != nil {
				t.Fatalf("stale quiz callback returned %v", err)
			}
			if bot.approves != tc.wantApprove || bot.declines != 0 || bot.bans != 0 {
				t.Fatalf("stale quiz actions = approve %d decline %d ban %d, want %d/0/0",
					bot.approves, bot.declines, bot.bans, tc.wantApprove)
			}
			if len(bot.callbackAnswers) != 1 {
				t.Fatalf("stale quiz acknowledgements = %d, want 1", len(bot.callbackAnswers))
			}
			if tc.wantStale {
				answer := bot.callbackAnswers[0]
				want := v.messages.Verification.Result.StaleQuestion.For(i18n.LangEN)
				if !answer.ShowAlert || answer.Text != want {
					t.Fatalf("stale quiz acknowledgement = %#v, want stale-question alert %q", answer, want)
				}
				if _, stillPending := v.pend[key]; !stillPending {
					t.Fatal("a stale quiz callback settled the replacement question")
				}
			}
		})
	}
}

type freshAdminGateway struct {
	*fakeVerifyBot
	fresh       bool
	cachedCalls int
	freshCalls  int
}

func (b *freshAdminGateway) CachedAdmin(context.Context, int64, int64) (bool, error) {
	b.cachedCalls++
	return true, nil
}

func (b *freshAdminGateway) FreshAdmin(context.Context, int64, int64) (bool, error) {
	b.freshCalls++
	return b.fresh, nil
}

func TestOnlyCurrentGroupAdministratorsSettleAdministratorCallbacks(t *testing.T) {
	const gid, uid, adminID = int64(-100900000201), int64(601), int64(701)
	for _, tc := range []struct {
		name        string
		action      string
		fresh       bool
		wantApprove int
		wantBan     int
	}{
		{name: "former administrator cannot approve", action: "pass"},
		{name: "current administrator can approve", action: "pass", fresh: true, wantApprove: 1},
		{name: "former administrator cannot kick", action: "ban"},
		{name: "current administrator can kick", action: "ban", fresh: true, wantBan: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestService(&settings.Config{GroupIDs: []int64{gid}, BanSeconds: 3600})
			key := pkey{gid: gid, uid: uid}
			v.pend[key] = &pending{nonce: "current", deadline: time.Now().Add(time.Hour)}
			base := newFakeVerifyBot()
			bot := &freshAdminGateway{fakeVerifyBot: base, fresh: tc.fresh}
			err := v.OnAdminAction(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), Update{
				CallbackQuery: &CallbackQuery{
					ID: "fresh-admin", From: User{ID: adminID},
					Data: AdminCallbackPrefix + tc.action + ":-100900000201:601:current",
				},
			})
			if err != nil {
				t.Fatalf("administrator callback returned %v", err)
			}
			if bot.freshCalls != 1 || bot.cachedCalls != 0 {
				t.Fatalf("administrator checks = fresh %d cached %d, want 1/0", bot.freshCalls, bot.cachedCalls)
			}
			if base.approves != tc.wantApprove || base.bans != tc.wantBan {
				t.Fatalf("administrator actions = approve %d ban %d, want %d/%d",
					base.approves, base.bans, tc.wantApprove, tc.wantBan)
			}
			_, pending := v.pend[key]
			if pending != !tc.fresh {
				t.Fatalf("administrator callback left pending = %t, want %t", pending, !tc.fresh)
			}
			if !tc.fresh {
				if len(base.callbackAnswers) != 1 {
					t.Fatalf("former administrator acknowledgements = %d, want 1", len(base.callbackAnswers))
				}
				answer := base.callbackAnswers[0]
				want := v.messages.Verification.Admin.OnlyGroupAdmin.For(i18n.LangZH)
				if !answer.ShowAlert || answer.Text != want {
					t.Fatalf("former administrator acknowledgement = %#v, want administrator-only alert %q", answer, want)
				}
			}
		})
	}
}

func TestAStaleAdministratorCallbackDoesNotQueryFreshAuthorization(t *testing.T) {
	const gid, uid, adminID = int64(-100900000202), int64(602), int64(702)
	for _, tc := range []struct {
		name          string
		nonce         string
		wantFreshCall int
		wantApprove   int
	}{
		{name: "replaced challenge", nonce: "stale"},
		{name: "current challenge", nonce: "current", wantFreshCall: 1, wantApprove: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestService(&settings.Config{GroupIDs: []int64{gid}, BanSeconds: 3600})
			key := pkey{gid: gid, uid: uid}
			v.pend[key] = &pending{nonce: "current", deadline: time.Now().Add(time.Hour)}
			base := newFakeVerifyBot()
			bot := &freshAdminGateway{fakeVerifyBot: base, fresh: true}
			err := v.OnAdminAction(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), Update{
				CallbackQuery: &CallbackQuery{
					ID: "nonce-before-admin", From: User{ID: adminID},
					Data: AdminCallbackPrefix + "pass:-100900000202:602:" + tc.nonce,
				},
			})
			if err != nil {
				t.Fatalf("stale administrator callback returned %v", err)
			}
			if bot.freshCalls != tc.wantFreshCall || bot.cachedCalls != 0 {
				t.Fatalf("stale administrator checks = fresh %d cached %d, want %d/0",
					bot.freshCalls, bot.cachedCalls, tc.wantFreshCall)
			}
			if base.approves != tc.wantApprove || base.bans != 0 {
				t.Fatalf("stale administrator actions = approve %d ban %d, want %d/0",
					base.approves, base.bans, tc.wantApprove)
			}
			if tc.wantFreshCall == 0 {
				if len(base.callbackAnswers) != 1 {
					t.Fatalf("stale administrator acknowledgements = %d, want 1", len(base.callbackAnswers))
				}
				answer := base.callbackAnswers[0]
				want := v.messages.Verification.Admin.AlreadyHandled.For(i18n.LangZH)
				if !answer.ShowAlert || answer.Text != want {
					t.Fatalf("stale administrator acknowledgement = %#v, want already-handled alert %q", answer, want)
				}
			}
			_, pending := v.pend[key]
			if pending != (tc.wantFreshCall == 0) {
				t.Fatalf("stale administrator pending = %t, want %t", pending, tc.wantFreshCall == 0)
			}
		})
	}
}

type stagedFreshAdminGateway struct {
	*fakeVerifyBot
	mu            sync.Mutex
	freshCalls    int
	firstReady    chan struct{}
	secondReady   chan struct{}
	releaseFirst  chan struct{}
	releaseSecond chan struct{}
}

func newStagedFreshAdminGateway() *stagedFreshAdminGateway {
	return &stagedFreshAdminGateway{
		fakeVerifyBot: newFakeVerifyBot(),
		firstReady:    make(chan struct{}, 1),
		secondReady:   make(chan struct{}, 1),
		releaseFirst:  make(chan struct{}, 1),
		releaseSecond: make(chan struct{}, 1),
	}
}

func (b *stagedFreshAdminGateway) FreshAdmin(context.Context, int64, int64) (bool, error) {
	b.mu.Lock()
	b.freshCalls++
	call := b.freshCalls
	b.mu.Unlock()
	switch call {
	case 1:
		b.firstReady <- struct{}{}
		<-b.releaseFirst
	case 2:
		b.secondReady <- struct{}{}
		<-b.releaseSecond
	}
	return true, nil
}

func TestConcurrentAdministratorCallbacksSettleOneChallengeOnce(t *testing.T) {
	const gid, uid, adminID = int64(-100900000203), int64(603), int64(703)
	for _, tc := range []struct {
		action      string
		wantApprove int
		wantBan     int
	}{
		{action: "pass", wantApprove: 1},
		{action: "ban", wantBan: 1},
	} {
		t.Run(tc.action, func(t *testing.T) {
			v := newTestService(&settings.Config{GroupIDs: []int64{gid}, BanSeconds: 3600})
			key := pkey{gid: gid, uid: uid}
			v.pend[key] = &pending{nonce: "current", deadline: time.Now().Add(time.Hour)}
			bot := newStagedFreshAdminGateway()
			release := func(ch chan<- struct{}) {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
			t.Cleanup(func() {
				release(bot.releaseFirst)
				release(bot.releaseSecond)
			})
			result := make(chan error, 2)
			call := func(id string) {
				update := Update{CallbackQuery: &CallbackQuery{
					ID: id, From: User{ID: adminID},
					Data: AdminCallbackPrefix + tc.action + ":-100900000203:603:current",
				}}
				result <- administratorCallbackResult(v, bot, update)
			}
			go call("first")
			waitForFreshAdmin(t, bot.firstReady, "first callback")
			go call("second")
			waitForFreshAdmin(t, bot.secondReady, "second callback")
			release(bot.releaseSecond)
			waitForAdminCallback(t, result, "second callback")
			release(bot.releaseFirst)
			waitForAdminCallback(t, result, "first callback")
			if bot.approves != tc.wantApprove || bot.bans != tc.wantBan {
				t.Fatalf("concurrent administrator actions = approve %d ban %d, want %d/%d",
					bot.approves, bot.bans, tc.wantApprove, tc.wantBan)
			}
			if tc.action == "ban" && bot.declines != 1 {
				t.Fatalf("concurrent kick declines = %d, want 1", bot.declines)
			}
			if bot.freshCalls != 2 || len(bot.callbackAnswers) != 2 {
				t.Fatalf("concurrent administrator checks/acknowledgements = %d/%d, want 2/2",
					bot.freshCalls, len(bot.callbackAnswers))
			}
			wantSecond := v.adminSays(gateRequest).CannotApprove.For(i18n.LangZH)
			if tc.action == "ban" {
				wantSecond = v.adminSays(gateRequest).AlreadyHandled.For(i18n.LangZH)
			}
			if got := bot.callbackAnswers[1].Text; got != wantSecond {
				t.Fatalf("second administrator acknowledgement = %q, want stale callback result %q", got, wantSecond)
			}
			if _, pending := v.pend[key]; pending {
				t.Fatal("concurrent administrator callbacks left a settled challenge pending")
			}
		})
	}
}

func administratorCallbackResult(v *Service, bot Gateway, update Update) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("concurrent administrator callback panicked instead of reporting a settled challenge")
		}
	}()
	return v.OnAdminAction(NewHandlerContext(context.Background(), bot), update)
}

func waitForFreshAdmin(t *testing.T, ready <-chan struct{}, callback string) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatalf("%s did not reach fresh administrator authorization", callback)
	}
}

func waitForAdminCallback(t *testing.T, result <-chan error, callback string) {
	t.Helper()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("%s returned %v", callback, err)
		}
	case <-time.After(time.Second):
		t.Fatalf("%s did not finish after authorization", callback)
	}
}

type administratorCallbackOrderGateway struct {
	*fakeVerifyBot
	events []string
}

func (b *administratorCallbackOrderGateway) AckResult(ctx context.Context, id string, result AckResult) error {
	b.events = append(b.events, "ack")
	return b.fakeVerifyBot.AckResult(ctx, id, result)
}

func (b *administratorCallbackOrderGateway) ApproveJoin(ctx context.Context, gid, uid int64) error {
	b.events = append(b.events, "approve")
	return b.fakeVerifyBot.ApproveJoin(ctx, gid, uid)
}

func (b *administratorCallbackOrderGateway) Ban(ctx context.Context, gid, uid int64, seconds int, revoke bool) error {
	b.events = append(b.events, "ban")
	return b.fakeVerifyBot.Ban(ctx, gid, uid, seconds, revoke)
}

func TestAdministratorCallbacksAcknowledgeBeforeExternalSettlement(t *testing.T) {
	const gid, uid, adminID = int64(-100900000204), int64(604), int64(704)
	for _, tc := range []struct {
		action string
		first  string
	}{
		{action: "pass", first: "approve"},
		{action: "ban", first: "ban"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			v := newTestService(&settings.Config{GroupIDs: []int64{gid}, BanSeconds: 3600})
			v.pend[pkey{gid: gid, uid: uid}] = &pending{nonce: "current", deadline: time.Now().Add(time.Hour)}
			base := newFakeVerifyBot()
			base.member = &ChatMemberAdministrator{Status: MemberStatusAdministrator}
			bot := &administratorCallbackOrderGateway{fakeVerifyBot: base}
			err := v.OnAdminAction(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), Update{
				CallbackQuery: &CallbackQuery{
					ID: "ack-order", From: User{ID: adminID},
					Data: AdminCallbackPrefix + tc.action + ":-100900000204:604:current",
				},
			})
			if err != nil {
				t.Fatalf("administrator acknowledgement callback returned %v", err)
			}
			if len(bot.events) < 2 || bot.events[0] != "ack" || bot.events[1] != tc.first {
				t.Fatalf("administrator callback order = %v, want acknowledgement before %s", bot.events, tc.first)
			}
		})
	}
}

func TestAdministratorApprovalReportsARequestAlreadySettledElsewhere(t *testing.T) {
	const gid, uid, adminID = int64(-100900000205), int64(605), int64(705)
	v := newTestService(&settings.Config{GroupIDs: []int64{gid}, BanSeconds: 3600})
	key := pkey{gid: gid, uid: uid}
	v.pend[key] = &pending{nonce: "current", deadline: time.Now().Add(time.Hour)}
	bot := newFakeVerifyBot()
	bot.approveErr = errors.New("user_already_participant")
	bot.memberByID = map[int64]ChatMember{
		adminID: &ChatMemberAdministrator{Status: MemberStatusAdministrator},
		uid:     &ChatMemberLeft{Status: MemberStatusLeft},
	}
	err := v.OnAdminAction(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), Update{
		CallbackQuery: &CallbackQuery{
			ID: "gone-approval", From: User{ID: adminID},
			Data: AdminCallbackPrefix + "pass:-100900000205:605:current",
		},
	})
	if err != nil {
		t.Fatalf("gone approval callback returned %v", err)
	}
	if bot.approves != 1 || bot.sends != 1 || bot.lastSendChat != gid {
		t.Fatalf("gone approval actions/messages = approve %d sends %d chat %d, want 1/1/%d",
			bot.approves, bot.sends, bot.lastSendChat, gid)
	}
	want := v.adminSays(gateRequest).AlreadyHandled.For(i18n.LangZH)
	if bot.lastSendText != want {
		t.Fatalf("gone approval group notice = %q, want already-handled result %q", bot.lastSendText, want)
	}
	if _, pending := v.pend[key]; pending {
		t.Fatal("a request already settled elsewhere remained pending")
	}
}

func TestAdministratorStorageFailuresLeaveChallengesUntouched(t *testing.T) {
	const gid, uid, adminID = int64(-100900000206), int64(606), int64(706)
	for _, tc := range []struct {
		action string
	}{
		{action: "pass"},
		{action: "ban"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			state := &errorTransitionStore{}
			v := newTestService(&settings.Config{GroupIDs: []int64{gid}, BanSeconds: 3600})
			v.statePath = "database"
			v.stateStore = state
			key := pkey{gid: gid, uid: uid}
			p := &pending{
				nonce: "current", deadline: time.Now().Add(time.Hour), epoch: 4, persistedPath: v.statePath,
			}
			v.pend[key] = p
			bot := newFakeVerifyBot()
			bot.member = &ChatMemberAdministrator{Status: MemberStatusAdministrator}
			err := v.OnAdminAction(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), Update{
				CallbackQuery: &CallbackQuery{
					ID: "storage-failure", From: User{ID: adminID},
					Data: AdminCallbackPrefix + tc.action + ":-100900000206:606:current",
				},
			})
			if err == nil {
				t.Fatal("administrator callback reported a database transition failure as success")
			}
			if state.calls != storeWriteMaxAttempts || bot.approves != 0 || bot.bans != 0 || bot.declines != 0 {
				t.Fatalf("storage failure calls/actions = %d/%d/%d/%d, want %d/0/0/0",
					state.calls, bot.approves, bot.bans, bot.declines, storeWriteMaxAttempts)
			}
			if current := v.pend[key]; current != p || p.done {
				t.Fatalf("storage failure pending = %+v, want the original live challenge", current)
			}
			if len(bot.callbackAnswers) != 1 || bot.callbackAnswers[0].Text != "" {
				t.Fatalf("storage failure acknowledgement = %#v, want one fast acknowledgement", bot.callbackAnswers)
			}
		})
	}
}
