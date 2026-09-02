package migrations

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAssessRollbackUsesMigrationCompatibility(t *testing.T) {
	compatible := AssessRollback(2, 1)
	if !compatible.CanRollback() || compatible.Reason != RollbackCompatible || compatible.MinimumCompatibleVersion != 1 {
		t.Fatalf("v2 to v1 assessment = %#v, want compatible with floor v1", compatible)
	}

	incompatible := AssessRollback(2, 0)
	if incompatible.CanRollback() || incompatible.Reason != RollbackIncompatible || incompatible.MinimumCompatibleVersion != 1 {
		t.Fatalf("v2 to v0 assessment = %#v, want incompatible below floor v1", incompatible)
	}
	if err := incompatible.Err(); !errors.Is(err, ErrRollbackBlocked) ||
		!strings.Contains(err.Error(), "v0") || !strings.Contains(err.Error(), "v1") {
		t.Fatalf("incompatible rollback explanation = %v, want displayable version details", err)
	}

	defaultFloor := AssessRollback(1, 0)
	if defaultFloor.CanRollback() || defaultFloor.Reason != RollbackIncompatible || defaultFloor.MinimumCompatibleVersion != 1 {
		t.Fatalf("v1 to v0 assessment = %#v, want dbutil's default floor v1", defaultFloor)
	}
}

func TestFetchAfterRollbackCheckBlocksBeforeRetrieval(t *testing.T) {
	fetches := 0
	assessment, err := FetchAfterRollbackCheck(context.Background(), 2, 0, func(context.Context) error {
		fetches++
		return nil
	})
	if !errors.Is(err, ErrRollbackBlocked) || assessment.Reason != RollbackIncompatible {
		t.Fatalf("unsafe fetch result = assessment:%#v error:%v", assessment, err)
	}
	if fetches != 0 {
		t.Fatalf("unsafe rollback called fetch %d times, want zero", fetches)
	}
}

func TestFetchAfterRollbackCheckRetrievesApprovedTarget(t *testing.T) {
	fetches := 0
	assessment, err := FetchAfterRollbackCheck(context.Background(), 2, 1, func(context.Context) error {
		fetches++
		return nil
	})
	if err != nil || !assessment.CanRollback() || fetches != 1 {
		t.Fatalf("safe fetch result = assessment:%#v fetches:%d error:%v", assessment, fetches, err)
	}
}
