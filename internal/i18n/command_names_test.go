package i18n

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

var legacyProductCommand = regexp.MustCompile(`(?:^|[^\w./])/(pkg|use|arm|bug|news|bbs)(?:[^a-z]|$)`)

func TestCatalogueUsesCanonicalCommandNames(t *testing.T) {
	for _, definition := range localeDefinitions {
		for _, subsystem := range []string{"bot", "verification", "panel", "moderate", "feed",
			"lookup_content", "lookup_distros", "lookup_packages"} {
			data, err := localeFiles.ReadFile(localeFilePath(definition.tag, subsystem))
			if err != nil {
				t.Fatalf("read %s/%s: %v", definition.tag, subsystem, err)
			}
			var tree any
			if err := json.Unmarshal(data, &tree); err != nil {
				t.Fatalf("parse %s/%s: %v", definition.tag, subsystem, err)
			}
			walkStrings(tree, "", func(path, value string) {
				for _, token := range []string{"{g}", "{ks}"} {
					if strings.Contains(value, token) {
						t.Errorf("%s/%s: %s retains removed edition token %q: %q",
							definition.tag, subsystem, path, token, value)
					}
				}
				for _, match := range legacyProductCommand.FindAllStringSubmatch(value, -1) {
					t.Errorf("%s/%s: %s retains legacy command /%s instead of its canonical name: %q",
						definition.tag, subsystem, path, match[1], value)
				}
			})
		}
	}
}
