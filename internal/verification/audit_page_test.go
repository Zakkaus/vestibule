package verification

import "sort"

func auditTestPage(source []ChallengeAuditRecord, page AuditPageRequest) []ChallengeAuditRecord {
	records := append([]ChallengeAuditRecord(nil), source...)
	for i := range records {
		records[i].Latest = true
		for j := range source {
			if source[j].Record.UserID == records[i].Record.UserID &&
				(source[j].SettledAt > records[i].SettledAt ||
					source[j].SettledAt == records[i].SettledAt && source[j].ID > records[i].ID) {
				records[i].Latest = false
			}
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].SettledAt == records[j].SettledAt {
			return records[i].ID > records[j].ID
		}
		return records[i].SettledAt > records[j].SettledAt
	})
	return records
}
