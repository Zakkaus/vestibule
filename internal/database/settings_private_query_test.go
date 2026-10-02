package database

import (
	"bytes"
	"context"
	"log"
	"testing"
)

func TestStoredPrivateQueryRateIsIgnored(t *testing.T) {
	const chatID int64 = -1009000000911
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ensureChat(ctx, db, chatID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE chat SET settings=$1, settings_revision=7 WHERE id=$2`,
		`{"private_query_per_min":0,"warn_limit":5}`, chatID); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	store := NewSettingsStore(db)
	records, err := store.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Revision != 7 || records[0].Overrides.WarnLimit == nil || *records[0].Overrides.WarnLimit != 5 {
		t.Fatalf("lost unrelated settings: %+v", records)
	}
	if !bytes.Contains(output.Bytes(), []byte("private_query_per_min")) {
		t.Fatal("missing migration warning")
	}
	if revision, written, err := store.CompareAndSwapSettings(chatID, 7, records[0].Overrides); err != nil || !written || revision != 8 {
		t.Fatalf("save revision=%d written=%v error=%v", revision, written, err)
	}
	var payload string
	if err := db.QueryRow(ctx, `SELECT settings FROM chat WHERE id=$1`, chatID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains([]byte(payload), []byte("private_query_per_min")) {
		t.Fatalf("obsolete field persisted: %s", payload)
	}
}
