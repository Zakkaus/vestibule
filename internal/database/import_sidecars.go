package database

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

const legacySidecarInstructions = "archive the whole legacy state and config tree; carry settings.json (including group-title metadata), antispam.json, feed cursors and other sidecars into the new STATE_DIRECTORY by hand before starting; --accept-unimported acknowledges this work, it does not import or back up these files"

// CheckLegacySidecars refuses to silently omit JSON state outside the five imported snapshots.
func CheckLegacySidecars(directory string, accept bool) ([]string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("inspect legacy state directory: %w", err)
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasSuffix(name, ".json") && !slices.Contains(legacyJSONNames, name) {
			names = append(names, name)
		}
	}
	if len(names) > 0 && !accept {
		return names, fmt.Errorf("unimported legacy files: %s; %s", strings.Join(names, ", "), legacySidecarInstructions)
	}
	return names, nil
}
