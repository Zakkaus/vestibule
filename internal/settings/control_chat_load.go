package settings

import "log"

// Existing stored claims precede file assignments; equal sources follow group load order.
func (s *Store) sanitizeLoadedControlChats() {
	seen := make(map[int64]int64)
	for _, stored := range []bool{true, false} {
		for i := range s.baseline.Groups {
			s.sanitizeLoadedControlChat(&s.baseline.Groups[i], stored, seen)
		}
		for _, registered := range s.state.RegisteredGroups {
			if _, configured := s.baselineByID[registered.ID]; configured {
				continue
			}
			baseline := s.baseline.Factory
			baseline.ID = registered.ID
			s.sanitizeLoadedControlChat(&baseline, stored, seen)
		}
	}
}

func (s *Store) sanitizeLoadedControlChat(baseline *GroupBaseline, stored bool, seen map[int64]int64) {
	record := s.state.Groups[baseline.ID]
	if (record.ControlChatID != nil) != stored {
		return
	}
	chatID := resolve(record.ControlChatID, baseline.ControlChatID).Value
	if chatID == 0 {
		return
	}
	if chatID < 0 && chatID != baseline.ID && seen[chatID] == 0 {
		seen[chatID] = baseline.ID
		return
	}
	log.Printf("settings: dropped control chat %d for group %d: invalid or already assigned", chatID, baseline.ID)
	if stored {
		record.ControlChatID = new(int64(0))
		s.state.Groups[baseline.ID] = record
	} else {
		baseline.ControlChatID.Value = 0
		if _, configured := s.baselineByID[baseline.ID]; configured {
			s.baselineByID[baseline.ID] = *baseline
		} else {
			record.ControlChatID = new(int64(0))
			s.state.Groups[baseline.ID] = record
		}
	}
}
