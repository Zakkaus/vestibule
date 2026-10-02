package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const cutoverGroup int64 = -1009000000711

type cutoverGateway struct {
	verification.Gateway
	released chan int64
	failure  error
	sends    int
}

func (g *cutoverGateway) Unmute(_ context.Context, _ int64, user int64) error {
	g.released <- user
	return g.failure
}
func (g *cutoverGateway) Send(context.Context, verification.OutgoingMessage) (int, error) {
	g.sends++
	return 0, nil
}

func importDroppedHolds(t *testing.T, db *database.Database) string {
	t.Helper()
	directory := t.TempDir()
	for _, name := range []string{"pending.json", "verifyfail.json", "agents.json", "heartbeat.json", "warns.json"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "state", name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "pending.json" {
			data = []byte(`[{"group_id":-1009000000711,"user_id":701,"nonce":"held-a","held":true},{"group_id":-1009000000711,"user_id":702,"nonce":"held-b","held":true},{"group_id":-1009000000711,"user_id":703,"nonce":"unheld"}]`)
		}
		if err = os.WriteFile(filepath.Join(directory, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	replayDrop(t, db, directory)
	return directory
}
func replayDrop(t *testing.T, db *database.Database, directory string) {
	t.Helper()
	report, err := database.ImportLegacyState(context.Background(), db, database.ImportOptions{StateDirectory: directory, BackupDirectory: filepath.Join(t.TempDir(), "backup"), Pending: database.PendingDrop})
	if err != nil {
		t.Fatal(err)
	}
	if report.PendingRows != 0 {
		t.Fatalf("restored dropped challenges: %d", report.PendingRows)
	}
}
func cutoverDatabase(t *testing.T) *database.Database {
	t.Helper()
	db, err := database.Open(context.Background(), database.Config{StateDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
func TestDroppedHoldsReleaseOnFirstStartAndReplayDoesNotRepeat(t *testing.T) {
	db := cutoverDatabase(t)
	directory := importDroppedHolds(t, db)
	gateway := &cutoverGateway{released: make(chan int64, 8)}
	cfg := &settings.Config{GroupIDs: []int64{cutoverGroup}, VerifyMode: settings.ModeKernel}
	store, err := settings.NewStore("", botTestSettingsBaseline(t, cfg), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := verification.New(store, gateway, database.NewVerificationStore(db), cfg, &i18n.Messages, nil, verification.Identity{}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Shutdown()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := startPendingActions(ctx, &services{database: db, verification: service, verificationGateway: gateway})
	got := map[int64]int{}
	for range 2 {
		select {
		case user := <-gateway.released:
			got[user]++
		case <-time.After(3 * time.Second):
			t.Fatal("first-start worker did not release held members")
		}
	}
	cancel()
	<-done
	if !reflect.DeepEqual(got, map[int64]int{701: 1, 702: 1}) {
		t.Fatalf("released members = %v", got)
	}
	var settled int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM pending_action WHERE kind='unrestrict' AND state='done'").Scan(&settled); err != nil {
		t.Fatal(err)
	}
	if settled != 2 {
		t.Fatalf("settled releases = %d", settled)
	}
	replayDrop(t, db, directory)
	executor := importedHoldExecutor{store: database.NewVerificationStore(db), gateway: gateway, owner: "replay", now: time.Now}
	executor.runOnce(context.Background())
	select {
	case user := <-gateway.released:
		t.Fatalf("replay released %d twice", user)
	default:
	}
}
func TestImportedHoldRetriesAndPermanentFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		failure  error
		attempts int
		state    string
		delay    time.Duration
	}{
		{"rate limit", &verification.GatewayError{Cause: errors.New("rate limit"), Kinds: verification.FailureRateLimited, RetryAfter: 17 * time.Second}, 0, "pending", 17 * time.Second},
		{"network", errors.New("offline"), 0, "pending", 5 * time.Second},
		{"unreachable", &verification.GatewayError{Cause: errors.New("forbidden"), Kinds: verification.FailureGroupUnreachable}, 0, "failed", 0},
		{"gone", &verification.GatewayError{Cause: errors.New("gone"), Kinds: verification.FailureApplicantGone}, 0, "failed", 0},
		{"budget", errors.New("offline"), 9, "failed", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := cutoverDatabase(t)
			importDroppedHolds(t, db)
			now := time.Now().Add(time.Second)
			if _, err := db.Exec(context.Background(), "UPDATE pending_action SET attempts=$1", tc.attempts); err != nil {
				t.Fatal(err)
			}
			gateway := &cutoverGateway{released: make(chan int64, 8), failure: tc.failure}
			executor := importedHoldExecutor{store: database.NewVerificationStore(db), gateway: gateway, owner: "retry", now: func() time.Time { return now }}
			executor.runOnce(context.Background())
			var state string
			var attempts int
			var next int64
			if err := db.QueryRow(context.Background(), "SELECT state, attempts, next_try_at FROM pending_action ORDER BY id LIMIT 1").Scan(&state, &attempts, &next); err != nil {
				t.Fatal(err)
			}
			if state != tc.state {
				t.Fatalf("action state = %s, want %s", state, tc.state)
			}
			if state == "pending" {
				if attempts != 1 || next != now.Add(tc.delay).Unix() {
					t.Fatalf("retry = %d/%d", attempts, next)
				}
				executor.runOnce(context.Background())
				if len(gateway.released) != 2 {
					t.Fatal("retried before due time")
				}
				now = now.Add(tc.delay)
				gateway.failure = nil
				executor.runOnce(context.Background())
				var count int
				if err := db.QueryRow(context.Background(), "SELECT count(*) FROM pending_action WHERE state='done'").Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != 2 {
					t.Fatalf("successful retries settled %d actions", count)
				}
			}
		})
	}
}
func TestPersistenceAlertsSuppressObservationAndExcludeReconciliation(t *testing.T) {
	for _, state := range []struct {
		name, contents string
		failure        bool
	}{
		{"reconciliation", `{"version":3,"owner_id":42,"registered_groups":[{"id":-1009000000711,"registered_by":42}],"groups":{}}`, false},
		{"validation", `{"version":3,"groups":{"-1009000000711":{"mute_seconds":0}}}`, true},
		{"read", `[`, true},
	} {
		for _, observe := range []bool{false, true} {
			t.Run(state.name+map[bool]string{false: "/live", true: "/observe"}[observe], func(t *testing.T) {
				db := cutoverDatabase(t)
				cfg := &settings.Config{GroupIDs: []int64{cutoverGroup}, VerifyMode: settings.ModeKernel, ObserveOnly: observe}
				path := filepath.Join(t.TempDir(), "settings.json")
				if err := os.WriteFile(path, []byte(state.contents), 0600); err != nil {
					t.Fatal(err)
				}
				store, err := settings.NewStore(path, botTestSettingsBaseline(t, cfg), nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				if (store.Persistence().LastError != nil) != state.failure {
					t.Fatalf("persistence = %+v", store.Persistence())
				}
				live := &cutoverGateway{}
				gateway, err := verificationGatewayForMode(context.Background(), cfg, db, live)
				if err != nil {
					t.Fatal(err)
				}
				alertPersistenceProblem(context.Background(), gateway, cfg, store)
				want := 0
				if state.failure && !observe {
					want = 1
				}
				if live.sends != want {
					t.Fatalf("live startup alerts = %d, want %d", live.sends, want)
				}
				assertObservedPersistenceAlert(t, db, state.failure && observe)
			})
		}
	}
}

func assertObservedPersistenceAlert(t *testing.T, db *database.Database, expected bool) {
	t.Helper()
	recorder, err := database.NewObservationStore(context.Background(), db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	actions, err := recorder.LoadObservedActions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if expected {
		if len(actions) != 1 || actions[0].Operation != verification.ObservedSend {
			raw, _ := json.Marshal(actions)
			t.Fatalf("observed alerts = %s", raw)
		}
	} else if len(actions) != 0 {
		t.Fatalf("unexpected observed alerts: %v", actions)
	}
}
