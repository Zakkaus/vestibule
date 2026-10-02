package verification

import "time"

func (v *Service) loadRecentPasses() error {
	now := v.wallNow()
	records, err := v.stateStore.LoadRecentPasses(v.statePath, now.Add(-recentPassWindow).Unix(), now.Unix())
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.passed == nil {
		v.passed = make(map[pkey]time.Time)
	}
	for _, record := range records {
		v.passed[pkey{record.GroupID, record.UserID}] = time.Unix(record.PassedAt, 0)
	}
	return nil
}
