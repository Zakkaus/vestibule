package i18n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func (f *Format) load(raw json.RawMessage, language Lang, path string) error {
	raw = bytes.TrimSpace(raw)
	if language == LangEN && len(raw) > 0 && raw[0] == '{' {
		var forms map[string]string
		if err := json.Unmarshal(raw, &forms); err != nil {
			return fmt.Errorf("%s: expected English one/other strings: %w", path, err)
		}
		if len(forms) != 2 || strings.TrimSpace(forms["one"]) == "" || strings.TrimSpace(forms["other"]) == "" {
			return fmt.Errorf("%s: expected non-empty English one and other forms", path)
		}
		f.one, f.localized[language] = forms["one"], forms["other"]
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%s: expected string: %w", path, err)
	}
	f.localized[language] = value
	return nil
}
