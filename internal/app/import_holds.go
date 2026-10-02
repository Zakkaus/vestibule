package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/verification"
)

type importedHoldExecutor struct {
	store   *database.VerificationStore
	gateway verification.Gateway
	owner   string
	now     func() time.Time
}

func (e *importedHoldExecutor) runOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	now := e.now()
	actions, err := e.store.ClaimImportedUnrestrict(e.owner, now.Unix(), now.Add(30*time.Second).Unix(), 32)
	if err != nil {
		log.Printf("imported hold action scan: %v", err)
		return
	}
	for _, action := range actions {
		if ctx.Err() != nil {
			return
		}
		var hold database.ImportedHold
		if err := json.Unmarshal([]byte(action.Payload), &hold); err != nil || hold.ChatID == 0 || hold.UserID <= 0 {
			_, err := e.store.FailAction("", action.ID, e.owner, e.now().Unix(), "invalid imported hold target")
			e.logStoreError(err)
			continue
		}
		e.finish(action, e.release(ctx, hold))
	}
}

func (e *importedHoldExecutor) release(ctx context.Context, hold database.ImportedHold) error {
	if hold.HoldUntil != 0 {
		member, err := e.gateway.Member(ctx, hold.ChatID, hold.UserID)
		if err == nil && member != nil {
			if until, comparable := member.RestrictionUntil(); comparable && until != hold.HoldUntil {
				return nil // An administrator or a newer verification owns this restriction.
			}
		}
	}
	return e.gateway.Unmute(ctx, hold.ChatID, hold.UserID)
}

func (e *importedHoldExecutor) finish(action verification.PendingAction, failure error) {
	now := e.now()
	var changed bool
	var err error
	if failure == nil {
		changed, err = e.store.CompleteAction("", action.ID, e.owner, now.Unix(), nil)
	} else {
		attempts := action.Attempts + 1
		var translated *verification.GatewayError
		errors.As(failure, &translated)
		permanent := translated != nil && translated.Kinds&(verification.FailureGroupUnreachable|verification.FailureApplicantGone) != 0
		if attempts >= 10 || permanent {
			changed, err = e.store.FailAction("", action.ID, e.owner, now.Unix(), failure.Error())
		} else {
			delay := min(5*time.Second<<uint(attempts-1), 5*time.Minute)
			if translated != nil && translated.RetryAfter > 0 {
				delay = translated.RetryAfter
			}
			changed, err = e.store.RetryAction("", action.ID, e.owner, attempts, now.Add(delay).Unix(), failure.Error())
		}
	}
	if err == nil && !changed {
		err = fmt.Errorf("imported hold action %s lost its lease", action.ID)
	}
	e.logStoreError(err)
}

func (e *importedHoldExecutor) logStoreError(err error) {
	if err != nil {
		log.Printf("imported hold action: %v", err)
	}
}
