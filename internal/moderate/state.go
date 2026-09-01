package moderate

import (
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

const (
	warnCounterMax          = 4096
	warningWriteMaxAttempts = 3
	warningWriteRetryDelay  = 10 * time.Millisecond
)

type warningKey struct {
	groupID int64
	userID  int64
}

type warningState struct {
	mu       sync.Mutex
	store    WarningStore
	counters map[warningKey]int
	loadErr  error
}

func newWarningState(store WarningStore) warningState {
	return warningState{
		store:    store,
		counters: make(map[warningKey]int),
	}
}

func (w *warningState) load() error {
	if w.store == nil {
		return nil
	}
	records, err := w.store.LoadWarnings()
	if err != nil {
		w.loadErr = err
		return err
	}
	w.loadErr = nil
	w.mu.Lock()
	for _, record := range records {
		if record.Count > 0 {
			w.counters[warningKey{groupID: record.GroupID, userID: record.UserID}] = record.Count
		}
	}
	w.pruneLocked()
	count := len(w.counters)
	w.mu.Unlock()
	if count > 0 {
		log.Printf("restored %d warning counter(s)", count)
	}
	return nil
}

func (w *warningState) save() error {
	if w.store == nil {
		return nil
	}
	if w.loadErr != nil {
		return fmt.Errorf("warning snapshot disabled after load failure: %w", w.loadErr)
	}
	var err error
	for attempt := 1; attempt <= warningWriteMaxAttempts; attempt++ {
		err = w.store.SaveWarnings(func() []WarningRecord {
			w.mu.Lock()
			defer w.mu.Unlock()
			records := make([]WarningRecord, 0, len(w.counters))
			for key, count := range w.counters {
				if count > 0 {
					records = append(records, WarningRecord{GroupID: key.groupID, UserID: key.userID, Count: count})
				}
			}
			return records
		})
		if err == nil {
			return nil
		}
		if attempt < warningWriteMaxAttempts {
			time.Sleep(time.Duration(attempt) * warningWriteRetryDelay)
		}
	}
	return fmt.Errorf("warning state write failed after %d attempts: %w", warningWriteMaxAttempts, err)
}

func (w *warningState) increment(groupID, userID int64) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := warningKey{groupID: groupID, userID: userID}
	w.counters[key]++
	count := w.counters[key]
	w.pruneLocked()
	return count
}

func (w *warningState) clear(groupID, userID int64) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	key := warningKey{groupID: groupID, userID: userID}
	count := w.counters[key]
	delete(w.counters, key)
	return count
}

// pruneLocked evicts the least severe counters first with deterministic key tie-breaking.
func (w *warningState) pruneLocked() {
	over := len(w.counters) - warnCounterMax
	if over <= 0 {
		return
	}
	type warning struct {
		key   warningKey
		count int
	}
	all := make([]warning, 0, len(w.counters))
	for key, count := range w.counters {
		all = append(all, warning{key: key, count: count})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].count != all[j].count {
			return all[i].count < all[j].count
		}
		if all[i].key.groupID != all[j].key.groupID {
			return all[i].key.groupID < all[j].key.groupID
		}
		return all[i].key.userID < all[j].key.userID
	})
	for _, item := range all[:over] {
		delete(w.counters, item.key)
	}
}
