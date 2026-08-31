package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/Zakkaus/vestibule/internal/verification"
)

const (
	agentTotalKey = "agent_total"
	heartbeatKey  = "last_online"
)

type pendingDelivery struct {
	GroupMessageID     int  `json:"group_message_id"`
	PrivateMessageID   int  `json:"private_message_id,omitempty"`
	ChallengeDelivered bool `json:"challenge_delivered,omitempty"`
}

// VerificationStore persists challenge rows and the remaining ancillary verification state.
type VerificationStore struct {
	db *Database
}

var _ verification.Store = (*VerificationStore)(nil)

func NewVerificationStore(db *Database) *VerificationStore {
	return &VerificationStore{db: db}
}

func (s *VerificationStore) LoadPending(_ string) ([]verification.PendingRecord, error) {
	rows, err := s.db.Query(context.Background(), `
		SELECT chat_id, user_id, payload, delivery, attempts, expires_at, epoch
		FROM challenge WHERE state='pending' ORDER BY chat_id, user_id`)
	if err != nil {
		return nil, fmt.Errorf("load pending challenges: %w", err)
	}
	defer rows.Close()
	var records []verification.PendingRecord
	for rows.Next() {
		record, err := scanPending(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("load pending challenges: %w", err)
	}
	return records, nil
}

func scanPending(row interface{ Scan(...any) error }) (verification.PendingRecord, error) {
	var record verification.PendingRecord
	var groupID, userID, deadline, epoch int64
	var attempts int
	var payload, deliveryJSON string
	if err := row.Scan(&groupID, &userID, &payload, &deliveryJSON, &attempts, &deadline, &epoch); err != nil {
		return record, fmt.Errorf("scan pending challenge: %w", err)
	}
	if err := json.Unmarshal([]byte(payload), &record); err != nil {
		return record, fmt.Errorf("decode pending payload for chat %d user %d: %w", groupID, userID, err)
	}
	var delivery pendingDelivery
	if err := json.Unmarshal([]byte(deliveryJSON), &delivery); err != nil {
		return record, fmt.Errorf("decode pending delivery for chat %d user %d: %w", groupID, userID, err)
	}
	record.GroupID = groupID
	record.UserID = userID
	record.Tries = attempts
	record.Deadline = deadline
	record.Epoch = uint64(epoch)
	record.GroupMsgID = delivery.GroupMessageID
	record.PrivateMsgID = delivery.PrivateMessageID
	record.ChallengeDelivered = delivery.ChallengeDelivered
	return record, nil
}

func (s *VerificationStore) InsertPending(_ string, record verification.PendingRecord) (bool, error) {
	return insertPending(context.Background(), s.db, record)
}

func (s *VerificationStore) UpdatePending(
	_ string,
	expected verification.PendingRef,
	record verification.PendingRecord,
) (bool, error) {
	if err := validatePendingReplacement(expected, record); err != nil {
		return false, err
	}
	payload, delivery, err := encodePending(record)
	if err != nil {
		return false, err
	}
	result, err := s.db.Exec(context.Background(), `
		UPDATE challenge
		   SET payload=$1, delivery=$2, attempts=$3, expires_at=$4, epoch=$5
		 WHERE id=$6 AND chat_id=$7 AND user_id=$8 AND state='pending' AND epoch=$9`,
		payload, delivery, record.Tries, record.Deadline, record.Epoch,
		challengeID(expected), expected.GroupID, expected.UserID, expected.Epoch)
	if err != nil {
		return false, fmt.Errorf("update pending challenge for chat %d user %d: %w", expected.GroupID, expected.UserID, err)
	}
	return changedRow(result)
}

func (s *VerificationStore) TransitionChallenge(_ string, transition verification.ChallengeTransition) (bool, error) {
	if err := validatePendingReplacement(transition.Expected, transition.Record); err != nil {
		return false, err
	}
	payload, delivery, err := encodePending(transition.Record)
	if err != nil {
		return false, err
	}
	var reason, settledAt, settledBy any
	if transition.To != verification.ChallengePending {
		settledAt = transition.SettledAt
		if transition.Reason != "" {
			reason = transition.Reason
		}
		if transition.SettledBy != 0 {
			settledBy = transition.SettledBy
		}
	}
	result, err := s.db.Exec(context.Background(), `
		UPDATE challenge
		   SET state=$1, payload=$2, delivery=$3, attempts=$4, expires_at=$5, epoch=$6,
		       reason=$7, settled_at=$8, settled_by=$9
		 WHERE id=$10 AND chat_id=$11 AND user_id=$12 AND state=$13 AND epoch=$14`,
		transition.To, payload, delivery, transition.Record.Tries, transition.Record.Deadline,
		transition.Record.Epoch, reason, settledAt, settledBy, challengeID(transition.Expected),
		transition.Expected.GroupID, transition.Expected.UserID, transition.From, transition.Expected.Epoch)
	if err != nil {
		return false, fmt.Errorf("transition challenge for chat %d user %d from %s to %s: %w",
			transition.Expected.GroupID, transition.Expected.UserID, transition.From, transition.To, err)
	}
	return changedRow(result)
}

func (s *VerificationStore) DeletePending(_ string, expected verification.PendingRef) (bool, error) {
	result, err := s.db.Exec(context.Background(), `
		DELETE FROM challenge
		 WHERE id=$1 AND chat_id=$2 AND user_id=$3 AND state='pending' AND epoch=$4`,
		challengeID(expected), expected.GroupID, expected.UserID, expected.Epoch)
	if err != nil {
		return false, fmt.Errorf("delete pending challenge for chat %d user %d: %w", expected.GroupID, expected.UserID, err)
	}
	return changedRow(result)
}

// replacePending runs only inside the one-time legacy import transaction; live writes are per row.
func replacePending(ctx context.Context, db *Database, records []verification.PendingRecord) error {
	if _, err := db.Exec(ctx, "DELETE FROM challenge WHERE state='pending'"); err != nil {
		return fmt.Errorf("clear pending challenges: %w", err)
	}
	for _, record := range records {
		inserted, err := insertPending(ctx, db, record)
		if err != nil {
			return err
		}
		if !inserted {
			return fmt.Errorf("duplicate imported pending challenge for chat %d user %d", record.GroupID, record.UserID)
		}
	}
	return nil
}

func insertPending(ctx context.Context, db *Database, record verification.PendingRecord) (bool, error) {
	if err := ensureChat(ctx, db, record.GroupID); err != nil {
		return false, err
	}
	payload, delivery, err := encodePending(record)
	if err != nil {
		return false, err
	}
	result, err := db.Exec(ctx, `
		INSERT INTO challenge
			(id, chat_id, user_id, state, kind, payload, delivery, attempts, expires_at, epoch)
		VALUES ($1, $2, $3, 'pending', 'rule', $4, $5, $6, $7, $8)
		ON CONFLICT DO NOTHING`,
		challengeID(record.Ref()), record.GroupID, record.UserID, payload, delivery, record.Tries, record.Deadline, record.Epoch)
	if err != nil {
		return false, fmt.Errorf("insert pending challenge for chat %d user %d: %w", record.GroupID, record.UserID, err)
	}
	return changedRow(result)
}

func encodePending(record verification.PendingRecord) (payload, delivery string, err error) {
	payloadJSON, err := json.Marshal(record)
	if err != nil {
		return "", "", fmt.Errorf("encode pending payload: %w", err)
	}
	deliveryJSON, err := json.Marshal(pendingDelivery{
		GroupMessageID: record.GroupMsgID, PrivateMessageID: record.PrivateMsgID,
		ChallengeDelivered: record.ChallengeDelivered,
	})
	if err != nil {
		return "", "", fmt.Errorf("encode pending delivery: %w", err)
	}
	return string(payloadJSON), string(deliveryJSON), nil
}

func validatePendingReplacement(expected verification.PendingRef, record verification.PendingRecord) error {
	if record.GroupID != expected.GroupID || record.UserID != expected.UserID || record.Nonce != expected.Nonce {
		return fmt.Errorf("pending replacement identity changed from %#v to %#v", expected, record.Ref())
	}
	return nil
}

func challengeID(ref verification.PendingRef) string {
	return strconv.FormatInt(ref.GroupID, 10) + ":" + strconv.FormatInt(ref.UserID, 10) + ":" + ref.Nonce
}

func changedRow(result sql.Result) (bool, error) {
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read affected challenge rows: %w", err)
	}
	return affected != 0, nil
}

func ensureChat(ctx context.Context, db *Database, chatID int64) error {
	_, err := db.Exec(ctx, "INSERT INTO chat (id, title) VALUES ($1, '') ON CONFLICT (id) DO NOTHING", chatID)
	if err != nil {
		return fmt.Errorf("ensure chat %d: %w", chatID, err)
	}
	return nil
}

func (s *VerificationStore) LoadFailures(_ string) ([]verification.FailureRecord, error) {
	rows, err := s.db.Query(context.Background(), `
		SELECT chat_id, user_id, count, last_at
		FROM verification_failure ORDER BY chat_id, user_id`)
	if err != nil {
		return nil, fmt.Errorf("load verification failures: %w", err)
	}
	defer rows.Close()
	var records []verification.FailureRecord
	for rows.Next() {
		var record verification.FailureRecord
		if err = rows.Scan(&record.GroupID, &record.UserID, &record.Count, &record.Last); err != nil {
			return nil, fmt.Errorf("scan verification failure: %w", err)
		}
		records = append(records, record)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("load verification failures: %w", err)
	}
	return records, nil
}

func (s *VerificationStore) SaveFailures(_ string, snapshot func() []verification.FailureRecord) error {
	snapshotWriteMu.Lock()
	defer snapshotWriteMu.Unlock()
	records := snapshot()
	return s.db.DoTxn(context.Background(), nil, func(ctx context.Context) error {
		return replaceFailures(ctx, s.db, records)
	})
}

func replaceFailures(ctx context.Context, db *Database, records []verification.FailureRecord) error {
	if _, err := db.Exec(ctx, "DELETE FROM verification_failure"); err != nil {
		return fmt.Errorf("clear verification failures: %w", err)
	}
	for _, record := range records {
		if err := ensureChat(ctx, db, record.GroupID); err != nil {
			return err
		}
		_, err := db.Exec(ctx, `
			INSERT INTO verification_failure (chat_id, user_id, count, last_at)
			VALUES ($1, $2, $3, $4)`, record.GroupID, record.UserID, record.Count, record.Last)
		if err != nil {
			return fmt.Errorf("insert verification failure for chat %d user %d: %w", record.GroupID, record.UserID, err)
		}
	}
	return nil
}

func (s *VerificationStore) LoadAgents(_ string) (verification.AgentTally, error) {
	ctx := context.Background()
	tally := verification.AgentTally{Counts: make(map[string]int)}
	rows, err := s.db.Query(ctx, "SELECT model, count FROM agent_tally ORDER BY model")
	if err != nil {
		return tally, fmt.Errorf("load agent tally: %w", err)
	}
	for rows.Next() {
		var model string
		var count int
		if err = rows.Scan(&model, &count); err != nil {
			_ = rows.Close()
			return tally, fmt.Errorf("scan agent tally: %w", err)
		}
		tally.Counts[model] = count
	}
	if err = rows.Close(); err != nil {
		return tally, fmt.Errorf("close agent tally rows: %w", err)
	}
	if err = rows.Err(); err != nil {
		return tally, fmt.Errorf("load agent tally: %w", err)
	}
	err = s.db.QueryRow(ctx, "SELECT value FROM verification_runtime WHERE key=$1", agentTotalKey).Scan(&tally.Total)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return tally, fmt.Errorf("load agent total: %w", err)
	}
	return tally, nil
}

func (s *VerificationStore) SaveAgents(_ string, snapshot func() verification.AgentTally) error {
	snapshotWriteMu.Lock()
	defer snapshotWriteMu.Unlock()
	tally := snapshot()
	return s.db.DoTxn(context.Background(), nil, func(ctx context.Context) error {
		return replaceAgents(ctx, s.db, tally)
	})
}

func replaceAgents(ctx context.Context, db *Database, tally verification.AgentTally) error {
	if _, err := db.Exec(ctx, "DELETE FROM agent_tally"); err != nil {
		return fmt.Errorf("clear agent tally: %w", err)
	}
	models := make([]string, 0, len(tally.Counts))
	for model := range tally.Counts {
		models = append(models, model)
	}
	sort.Strings(models)
	for _, model := range models {
		if _, err := db.Exec(ctx, "INSERT INTO agent_tally (model, count) VALUES ($1, $2)", model, tally.Counts[model]); err != nil {
			return fmt.Errorf("insert agent tally %q: %w", model, err)
		}
	}
	return upsertRuntimeValue(ctx, db, agentTotalKey, int64(tally.Total))
}

func (s *VerificationStore) LoadHeartbeat(_ string) (verification.HeartbeatRecord, error) {
	var heartbeat verification.HeartbeatRecord
	err := s.db.QueryRow(context.Background(), "SELECT value FROM verification_runtime WHERE key=$1", heartbeatKey).
		Scan(&heartbeat.LastOnline)
	if errors.Is(err, sql.ErrNoRows) {
		return heartbeat, nil
	}
	if err != nil {
		return heartbeat, fmt.Errorf("load heartbeat: %w", err)
	}
	return heartbeat, nil
}

func (s *VerificationStore) SaveHeartbeat(_ string, heartbeat verification.HeartbeatRecord) error {
	snapshotWriteMu.Lock()
	defer snapshotWriteMu.Unlock()
	return upsertRuntimeValue(context.Background(), s.db, heartbeatKey, heartbeat.LastOnline)
}

func upsertRuntimeValue(ctx context.Context, db *Database, key string, value int64) error {
	_, err := db.Exec(ctx, `
		INSERT INTO verification_runtime (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value=excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("save verification runtime value %q: %w", key, err)
	}
	return nil
}
