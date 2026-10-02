package database

import "github.com/Zakkaus/vestibule/internal/verification"

func (*VerificationJSONStore) SettlementActionCurrent(
	string, string, string, verification.PendingRef, verification.ChallengeState,
) (bool, error) {
	return true, nil
}

func (*VerificationJSONStore) LoadRecentPasses(string, int64, int64) ([]verification.RecentPassRecord, error) {
	return nil, nil
}

func (*VerificationJSONStore) SupersedeGroupSettlements(string, int64, int64) ([]verification.PendingRecord, error) {
	return nil, nil
}
