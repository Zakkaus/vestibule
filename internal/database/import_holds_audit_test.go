package database_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/api"
	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const dropAuditGroup int64 = -1009000000711

type dropAuditGateway struct {
	verification.Gateway
	unbanned int
}

func (g *dropAuditGateway) Unban(context.Context, int64, int64, bool) error {
	g.unbanned++
	return nil
}

func (*dropAuditGateway) CachedAdmin(context.Context, int64, int64) (bool, error) {
	return true, nil
}

func (*dropAuditGateway) FreshRights(context.Context, int64, int64) (verification.GroupRights, error) {
	return verification.GroupRights{CanRestrictMembers: true}, nil
}

func TestDroppedHoldAnchorsDoNotPoisonAuditAndUndoHTTP(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(map[bool]string{false: "queued", true: "released"}[completed], func(t *testing.T) {
			ctx := context.Background()
			db, err := database.Open(ctx, database.Config{StateDirectory: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			state := database.NewVerificationStore(db)
			ban := verification.PendingRecord{GroupID: dropAuditGroup, UserID: 701, Nonce: "manual-ban", Deadline: 90, Epoch: 1}
			inserted, err := state.InsertPending("", ban)
			if err != nil || !inserted {
				t.Fatalf("insert manual decision = %t, %v", inserted, err)
			}
			changed, err := state.TransitionChallenge("", verification.ChallengeTransition{
				Expected: ban.Ref(), Record: ban, From: verification.ChallengePending,
				To: verification.ChallengeBanned, SettledAt: 100, SettledBy: 9,
			})
			if err != nil || !changed {
				t.Fatalf("manual ban = %t, %v", changed, err)
			}
			directory := t.TempDir()
			if err = os.WriteFile(filepath.Join(directory, "pending.json"), []byte(`[{"group_id":-1009000000711,"user_id":701,"nonce":"held","held":true,"hold_until":2000000000}]`), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err = database.ImportLegacyState(ctx, db, database.ImportOptions{
				StateDirectory: directory, BackupDirectory: filepath.Join(t.TempDir(), "backup"), Pending: database.PendingDrop,
			}); err != nil {
				t.Fatal(err)
			}
			if completed {
				now := time.Now().Add(time.Second).Unix()
				actions, err := state.ClaimImportedUnrestrict("release-worker", now, now+30, 10)
				if err != nil || len(actions) != 1 {
					t.Fatalf("release anchor reference = %+v, %v", actions, err)
				}
				changed, err := state.CompleteAction("", actions[0].ID, "release-worker", now, nil)
				if err != nil || !changed {
					t.Fatalf("settle release = %t, %v", changed, err)
				}
			}
			assertDropAuditHTTP(t, db)
		})
	}
}

func assertDropAuditHTTP(t *testing.T, db *database.Database) {
	t.Helper()
	gateway := &dropAuditGateway{}
	cfg := &settings.Config{GroupIDs: []int64{dropAuditGroup}, VerifyMode: settings.ModeKernel}
	baseline, err := settings.LoadBaseline(filepath.Join(t.TempDir(), "missing.json"), cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore("", baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := verification.New(store, gateway, database.NewVerificationStore(db), cfg, &i18n.Messages, nil, verification.Identity{}, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	manager, err := auth.New(auth.Config{
		BotToken: "1:synthetic-cutover-token", AdminChecker: gateway, RightsChecker: gateway,
		OperatorAllowed: func(id int64) bool { return id == 9 },
	})
	if err != nil {
		t.Fatal(err)
	}
	link, _, err := manager.IssueOperatorLink(9)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := manager.RedeemOperatorLink(link)
	if err != nil {
		t.Fatal(err)
	}
	cookies := httptest.NewRecorder()
	manager.SetCookies(cookies, grant)
	server := httptest.NewServer(api.New(api.Config{Authenticator: manager, Verification: service}).Handler())
	t.Cleanup(server.Close)
	auditPath := fmt.Sprintf("/api/chats/%d/audit", dropAuditGroup)
	banID := fmt.Sprintf("%d:701:manual-ban", dropAuditGroup)
	list := dropAuditRequest(t, server.URL+auditPath, http.MethodGet, cookies.Result().Cookies(), grant.CSRFToken)
	defer list.Body.Close()
	if list.StatusCode != http.StatusOK {
		t.Fatalf("audit HTTP status = %d, want 200", list.StatusCode)
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err = json.NewDecoder(list.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != banID {
		t.Fatalf("audit contains a synthetic release instead of only the manual ban: %+v", page.Items)
	}
	undo := dropAuditRequest(t, server.URL+auditPath+"/"+banID+"/undo", http.MethodPost, cookies.Result().Cookies(), grant.CSRFToken)
	defer undo.Body.Close()
	if undo.StatusCode != http.StatusOK || gateway.unbanned != 1 {
		t.Fatalf("undo HTTP status = %d, unbans = %d; want 200/1", undo.StatusCode, gateway.unbanned)
	}
}

func dropAuditRequest(t *testing.T, endpoint, method string, cookies []*http.Cookie, csrf string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	request.Header.Set("X-CSRF-Token", csrf)
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
