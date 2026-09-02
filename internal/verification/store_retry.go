package verification

import (
	"fmt"
	"time"
)

const (
	storeWriteMaxAttempts = 3
	storeWriteRetryDelay  = 10 * time.Millisecond
)

type storeWriteObserver interface {
	RecordDatabaseWrite(success bool)
}

func retryStoreWrite(observer storeWriteObserver, write func() error) error {
	var err error
	for attempt := 1; attempt <= storeWriteMaxAttempts; attempt++ {
		if err = write(); err == nil {
			if observer != nil {
				observer.RecordDatabaseWrite(true)
			}
			return nil
		}
		pauseBeforeStoreRetry(attempt)
	}
	if observer != nil {
		observer.RecordDatabaseWrite(false)
	}
	return fmt.Errorf("state write failed after %d attempts: %w", storeWriteMaxAttempts, err)
}

func retryStoreChange(write func() (bool, error)) (bool, error) {
	var err error
	for attempt := 1; attempt <= storeWriteMaxAttempts; attempt++ {
		var changed bool
		changed, err = write()
		if err == nil {
			return changed, nil
		}
		pauseBeforeStoreRetry(attempt)
	}
	return false, fmt.Errorf("state write failed after %d attempts: %w", storeWriteMaxAttempts, err)
}

func pauseBeforeStoreRetry(attempt int) {
	if attempt < storeWriteMaxAttempts {
		time.Sleep(time.Duration(attempt) * storeWriteRetryDelay)
	}
}
