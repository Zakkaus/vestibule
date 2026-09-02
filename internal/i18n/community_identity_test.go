package i18n

import (
	"encoding/json"
	"regexp"
	"testing"
)

var communityIdentityClaim = regexp.MustCompile(`Gentoo-zh Community|Gentoo 中文社[区群]|Gentoo Chinese Community|gentoozh\.org`)

func TestCatalogueClaimsNoSpecificCommunityIdentity(t *testing.T) {
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
				if communityIdentityClaim.MatchString(value) {
					t.Errorf("%s/%s: %s claims a specific community identity: %q",
						definition.tag, subsystem, path, value)
				}
			})
		}
	}
}

func walkStrings(node any, path string, visit func(path, value string)) {
	switch typed := node.(type) {
	case map[string]any:
		for key, child := range typed {
			next := key
			if path != "" {
				next = path + "." + key
			}
			walkStrings(child, next, visit)
		}
	case []any:
		for _, child := range typed {
			walkStrings(child, path, visit)
		}
	case string:
		visit(path, typed)
	}
}
