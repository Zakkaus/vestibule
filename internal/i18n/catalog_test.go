package i18n

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var (
	textType       = reflect.TypeFor[Text]()
	formatType     = reflect.TypeFor[Format]()
	stringListType = reflect.TypeFor[StringList]()
)

func TestFromTelegram(t *testing.T) {
	tests := map[string]Lang{
		"zh-hans":   LangZH,
		"zh-CN":     LangZH,
		"zh":        LangZH,
		"zh-sg":     LangZH,
		" zh-Hant ": LangZHHant,
		"zh-hant":   LangZHHant,
		"zh-TW":     LangZHHant,
		"zh-hk":     LangZHHant,
		"zh-MO":     LangZHHant,
		"yue":       LangZHHant,
		"en":        LangEN,
		"en-US":     LangEN,
		"ru":        LangEN,
		"ja":        LangEN,
		"":          LangEN,
	}
	for tag, want := range tests {
		if got := FromTelegram(tag); got != want {
			t.Errorf("FromTelegram(%q) = %s, want %s; an applicant would receive the wrong catalogue", tag, got, want)
		}
	}
}

func TestFromRequester(t *testing.T) {
	for code, want := range map[string]Lang{
		"en":      LangEN,
		"en-US":   LangEN,
		"en_US":   LangEN,
		"zh-CN":   LangZH,
		"zh-Hant": LangZHHant,
		"yue-HK":  LangZHHant,
		"":        LangZHHant,
		"fr":      LangZHHant,
	} {
		if got := FromRequester(code, LangZHHant); got != want {
			t.Errorf("FromRequester(%q, zh-Hant) = %v, want %v; a requester would receive the wrong catalogue", code, got, want)
		}
	}
}

func TestFromRequesterKeepsConfiguredFallbackForUnsupportedLanguage(t *testing.T) {
	for _, fallback := range Languages() {
		if got := FromRequester("fr-CA", fallback); got != fallback {
			t.Errorf("FromRequester(%q, %s) = %s, want %s; an unsupported requester language would replace the group's configured catalogue", "fr-CA", fallback, got, fallback)
		}
	}
}

func TestFromRequesterServesCantoneseInTraditionalChinese(t *testing.T) {
	const code = "yue-HK"
	if got := FromRequester(code, LangEN); got != LangZHHant {
		t.Fatalf("FromRequester(%q, en) = %s, want zh-Hant; Cantonese requester would receive the fallback catalogue", code, got)
	}
}

func TestFromRequesterNormalizesTagsBeforeMatching(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		fallback Lang
		want     Lang
	}{
		{name: "upper-case Traditional Chinese", code: "ZH-TW", fallback: LangZH, want: LangZHHant},
		{name: "surrounding whitespace", code: " en-US ", fallback: LangZH, want: LangEN},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := FromRequester(test.code, test.fallback); got != test.want {
				t.Errorf("FromRequester(%q, %s) = %s, want %s; requester would receive the fallback catalogue", test.code, test.fallback, got, test.want)
			}
		})
	}
}

func TestFromStored(t *testing.T) {
	tests := map[string]Lang{
		"":          LangZH,
		"zh":        LangZH,
		"zh-hans":   LangZH,
		" zh-Hant ": LangZHHant,
		"zh-Hant":   LangZHHant,
		"zh-hant":   LangZHHant,
		"en":        LangEN,
		"unknown":   LangZH,
	}
	for tag, want := range tests {
		if got := FromStored(tag); got != want {
			t.Errorf("FromStored(%q) = %s, want %s; a persisted language would render the wrong catalogue", tag, got, want)
		}
	}
}

func TestLangString(t *testing.T) {
	tests := map[Lang]string{
		LangZH:     "zh",
		LangZHHant: "zh-Hant",
		LangEN:     "en",
	}
	for language, want := range tests {
		if got := language.String(); got != want {
			t.Errorf("Lang(%d).String() = %q, want %q", language, got, want)
		}
	}
}

func TestUnsupportedLanguageUsesSimplifiedChineseCatalogue(t *testing.T) {
	invalid := Lang(langCount)
	if got, want := invalid.String(), LangZH.String(); got != want {
		t.Errorf("Lang(%d).String() = %q, want %q; an invalid persisted language would not fall back to Simplified Chinese", invalid, got, want)
	}
	if got, want := Messages.Verification.Mode.Kernel.For(invalid), Messages.Verification.Mode.Kernel.For(LangZH); got != want {
		t.Errorf("Text.For(%d) = %q, want %q; an invalid language would not render the default catalogue", invalid, got, want)
	}
	if got, want := Messages.Verification.Duration.Days.Render(invalid, 2), Messages.Verification.Duration.Days.Render(LangZH, 2); got != want {
		t.Errorf("Format.Render(%d) = %q, want %q; an invalid language would not render the default catalogue", invalid, got, want)
	}
	if got, want := Messages.Verification.Input.OtherOSPhrases.For(invalid), Messages.Verification.Input.OtherOSPhrases.For(LangZH); !slices.Equal(got, want) {
		t.Errorf("StringList.For(%d) = %q, want %q; an invalid language would not render the default catalogue", invalid, got, want)
	}
}

func TestCatalogComplete(t *testing.T) {
	visitCatalog(reflect.ValueOf(Messages), "Messages", func(path string, value reflect.Value) {
		switch value.Type() {
		case textType, formatType:
			for locale, text := range localizedValues(value) {
				location := catalogEntry(path, locale)
				if text == "" {
					t.Errorf("%s is empty", location)
				}
				if value.Type() == textType && looksFormatted(text) {
					t.Errorf("%s is Text but contains a format directive", location)
				}
			}
		case stringListType:
			for locale, entries := range localizedStringValues(value) {
				location := catalogEntry(path, locale)
				if len(entries) == 0 {
					t.Errorf("%s is empty", location)
				}
				for i, entry := range entries {
					if entry == "" {
						t.Errorf("%s[%d] is empty", location, i)
					}
				}
			}
		}
	})
}

func TestSetupRecoveryCatalogNamesImplementedRestartAction(t *testing.T) {
	for _, definition := range localeDefinitions {
		data, err := localeFiles.ReadFile(localeFilePath(definition.tag, "moderate"))
		if err != nil {
			t.Fatal(err)
		}
		var raw struct {
			Setup map[string]string `json:"setup"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatal(err)
		}
		recovery, ok := raw.Setup["restart"]
		if !ok || recovery == "" {
			t.Errorf("%s setup catalogue has no restart recovery action", definition.tag)
		}
		if _, stale := raw.Setup["recheck"]; stale {
			t.Errorf("%s setup catalogue still names the nonexistent recheck action", definition.tag)
		}
	}
}

func TestFormatPlaceholdersMatchLocales(t *testing.T) {
	visitCatalog(reflect.ValueOf(Messages), "Messages", func(path string, value reflect.Value) {
		if value.Type() != formatType {
			return
		}
		values := localizedValues(value)
		want, err := indexedPlaceholders(values[LangZH])
		if err != nil {
			t.Errorf("%s has an invalid format: %v", catalogEntry(path, LangZH), err)
			return
		}
		if len(want) == 0 {
			t.Errorf("%s is Format but has no placeholders", catalogEntry(path, LangZH))
		}
		for _, locale := range Languages() {
			if locale == LangZH {
				continue
			}
			got, err := indexedPlaceholders(values[locale])
			if err != nil {
				t.Errorf("%s has an invalid format: %v", catalogEntry(path, locale), err)
				continue
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s placeholders = %v, want %v", catalogEntry(path, locale), got, want)
			}
		}
	})
}

func visitCatalog(value reflect.Value, path string, visit func(string, reflect.Value)) {
	visit(path, value)
	if value.Type() == textType || value.Type() == formatType || value.Type() == stringListType {
		return
	}
	switch value.Kind() {
	case reflect.Struct:
		typ := value.Type()
		for i := range value.NumField() {
			visitCatalog(value.Field(i), path+"."+typ.Field(i).Name, visit)
		}
	case reflect.Array, reflect.Slice:
		for i := range value.Len() {
			visitCatalog(value.Index(i), fmt.Sprintf("%s[%d]", path, i), visit)
		}
	}
}

func catalogEntry(path string, locale Lang) string {
	subsystem, key := catalogPath(path)
	return fmt.Sprintf(
		"subsystem %s file %s key %s",
		subsystem,
		localeFilePath(locale.String(), subsystem),
		key,
	)
}

func catalogPath(path string) (string, string) {
	parts := strings.Split(strings.TrimPrefix(path, "Messages."), ".")
	subsystem := fieldKey(parts[0])
	for i := 1; i < len(parts); i++ {
		parts[i-1] = catalogJSONSegment(parts[i])
	}
	return subsystem, strings.Join(parts[:len(parts)-1], ".")
}

func catalogJSONSegment(segment string) string {
	if bracket := strings.IndexByte(segment, '['); bracket >= 0 {
		return fieldKey(segment[:bracket]) + segment[bracket:]
	}
	return fieldKey(segment)
}

func localizedValues(value reflect.Value) map[Lang]string {
	localized := value.Field(0)
	values := make(map[Lang]string, len(Languages()))
	for _, language := range Languages() {
		values[language] = localized.Index(int(language)).String()
	}
	return values
}

func localizedStringValues(value reflect.Value) map[Lang][]string {
	localized := value.Field(0)
	values := make(map[Lang][]string, len(Languages()))
	for _, language := range Languages() {
		values[language] = reflectedStrings(localized.Index(int(language)))
	}
	return values
}

func reflectedStrings(value reflect.Value) []string {
	items := make([]string, value.Len())
	for i := range value.Len() {
		items[i] = value.Index(i).String()
	}
	return items
}

func looksFormatted(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] != '%' || i+1 == len(value) {
			continue
		}
		next := value[i+1]
		if next == '%' {
			i++
			continue
		}
		if next == '[' || strings.ContainsRune("vTtbcdoOxXUeEfFgGspqx", rune(next)) {
			return true
		}
	}
	return false
}

func indexedPlaceholders(value string) ([]string, error) {
	var placeholders []string
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			continue
		}
		if i+1 < len(value) && value[i+1] == '%' {
			i++
			continue
		}
		if i+1 >= len(value) || value[i+1] != '[' {
			return nil, fmt.Errorf("implicit directive at byte %d", i)
		}
		start := i + 2
		end := start
		for end < len(value) && value[end] >= '0' && value[end] <= '9' {
			end++
		}
		if end == start || end >= len(value) || value[end] != ']' {
			return nil, fmt.Errorf("invalid argument index at byte %d", i)
		}
		index, err := strconv.Atoi(value[start:end])
		if err != nil || index < 1 {
			return nil, fmt.Errorf("invalid argument index at byte %d", i)
		}
		verbPos := end + 1
		if verbPos >= len(value) || !strings.ContainsRune("vTtbcdoOxXUeEfFgGspqx", rune(value[verbPos])) {
			return nil, fmt.Errorf("unsupported directive at byte %d", i)
		}
		placeholders = append(placeholders, fmt.Sprintf("%d:%c", index, value[verbPos]))
		i = verbPos
	}
	sort.Strings(placeholders)
	return placeholders, nil
}
