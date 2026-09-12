package settings

import (
	"os"
	"path/filepath"
	"testing"
)

const factoryQuestionsTestGroup int64 = -1009000002401

func writeFactoryQuestionsConfig(t *testing.T, bank string) string {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"factory_questions_file":"factory-bank.json"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory-bank.json"), []byte(bank), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath
}

const customFactoryQuestions = `{
  "questions": [
    {"q":"Which shape has three sides?","options":["triangle","square"],"answer":0}
  ],
  "fallback_questions": [
    {"q":"What is two plus three?","answers":["5","five"]}
  ]
}`

func TestLoadConfigFactoryQuestionsFileUsesConfigDirectory(t *testing.T) {
	configPath := writeFactoryQuestionsConfig(t, customFactoryQuestions)
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	baseline, err := LoadBaseline(configPath, cfg)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	if got := baseline.Factory.Questions; got.Source != SourceUserFile || len(got.Value) != 1 || got.Value[0].Q != "Which shape has three sides?" {
		t.Fatalf("factory quiz bank = %+v, want the relative factory file bank from the user file", got)
	}
	if got := baseline.Factory.FallbackQuestions; got.Source != SourceUserFile || len(got.Value) != 1 || got.Value[0].Q != "What is two plus three?" {
		t.Fatalf("factory fallback bank = %+v, want the relative factory file bank from the user file", got)
	}
	if got := baseline.Factory.FallbackBuiltin; !got.Value {
		t.Fatalf("supplied deployment bank selected per-chat fallback mode: %+v", got)
	}
}

func TestFactoryQuestionsFileMissingAndMalformedRefuseStartup(t *testing.T) {
	tests := []struct {
		name string
		bank string
	}{
		{name: "missing", bank: ""},
		{name: "malformed", bank: `{"questions":[`},
		{name: "trailing document", bank: `{"questions":[{"q":"q","options":["a","b"],"answer":0}]} {}`},
		{name: "unknown field", bank: `{"questions":[{"q":"q","options":["a","b"],"answer":0}],"extra":true}`},
		{name: "unknown question field", bank: `{"questions":[{"q":"q","options":["a","b"],"answer":0,"extra":true}]}`},
		{name: "empty quiz bank", bank: `{"questions":[]}`},
		{name: "blank quiz prompt", bank: `{"questions":[{"q":"  ","options":["a","b"],"answer":0}]}`},
		{name: "null quiz with valid fallback", bank: `{"questions":null,"fallback_questions":[{"q":"q","answers":["a"]}]}`},
		{name: "null fallback with valid quiz", bank: `{"questions":[{"q":"q","options":["a","b"],"answer":0}],"fallback_questions":null}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(configPath, []byte(`{"factory_questions_file":"bank.json"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			if test.bank != "" {
				if err := os.WriteFile(filepath.Join(filepath.Dir(configPath), "bank.json"), []byte(test.bank), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := LoadConfig(configPath); err == nil {
				t.Fatal("LoadConfig accepted an invalid factory question bank")
			}
		})
	}
}

func TestRegisteredGroupInheritsFactoryQuestionsAndDetachedData(t *testing.T) {
	configPath := writeFactoryQuestionsConfig(t, customFactoryQuestions)
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := LoadBaseline(configPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(filepath.Join(t.TempDir(), "settings.json"), baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	registration := store.Registrations()
	registration.OwnerID = 42
	registration.RegisteredGroups = []RegisteredGroup{{ID: factoryQuestionsTestGroup, RegisteredBy: 42}}
	if _, err := store.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatal(err)
	}
	group, ok := store.Settings(factoryQuestionsTestGroup)
	if !ok {
		t.Fatal("registered group did not become readable")
	}
	if got := group.Questions(); got.Source != SourceUserFile || len(got.Value) != 1 || got.Value[0].Q != "Which shape has three sides?" {
		t.Fatalf("registered quiz bank = %+v, want inherited factory file value", got)
	}
	if got := group.FallbackQuestions(); got.Source != SourceUserFile || len(got.Value) != 1 || got.Value[0].Q != "What is two plus three?" {
		t.Fatalf("registered fallback bank = %+v, want inherited factory file value", got)
	}
	questions := group.Questions().Value
	questions[0].Options[0] = "changed"
	fallback := group.FallbackQuestions().Value
	fallback[0].Answers[0] = "changed"
	reloaded, _ := store.Settings(factoryQuestionsTestGroup)
	if reloaded.Questions().Value[0].Options[0] != "triangle" || reloaded.FallbackQuestions().Value[0].Answers[0] != "5" {
		t.Fatal("registered group exposed mutable factory question data")
	}
}

func TestFactoryQuestionChatOverrideAndNullRestoreKeepFileSource(t *testing.T) {
	configPath := writeFactoryQuestionsConfig(t, customFactoryQuestions)
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := LoadBaseline(configPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	groupBaseline := cloneGroupBaseline(baseline.Factory)
	groupBaseline.ID = factoryQuestionsTestGroup
	store, err := NewStore("", SettingsBaseline{
		Factory: baseline.Factory,
		Groups:  []GroupBaseline{groupBaseline},
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	group, _ := store.Settings(factoryQuestionsTestGroup)
	overrideQuestions := []Question{{Q: "Override?", Options: []string{"yes", "no"}, Answer: 0}}
	overrideFallback := []ShortQuestion{{Q: "Override fallback?", Answers: []string{"yes"}}}
	overrides := group.Overrides()
	overrides.Questions = &overrideQuestions
	overrides.FallbackQuestions = &overrideFallback
	builtin := false
	overrides.FallbackBuiltin = &builtin
	if _, err := store.Update(group.ID(), group.Revision(), overrides); err != nil {
		t.Fatal(err)
	}
	group, _ = store.Settings(factoryQuestionsTestGroup)
	if group.Questions().Source != SourceChatOverride || group.FallbackQuestions().Source != SourceChatOverride {
		t.Fatalf("override sources = %v / %v, want chat override", group.Questions().Source, group.FallbackQuestions().Source)
	}
	restore := group.Overrides()
	restore.Questions = nil
	restore.FallbackQuestions = nil
	restore.FallbackBuiltin = nil
	if _, err := store.Update(group.ID(), group.Revision(), restore); err != nil {
		t.Fatal(err)
	}
	group, _ = store.Settings(factoryQuestionsTestGroup)
	if group.Questions().Source != SourceUserFile || group.FallbackQuestions().Source != SourceUserFile {
		t.Fatalf("restored sources = %v / %v, want user file", group.Questions().Source, group.FallbackQuestions().Source)
	}
}

func TestFactoryDefaultModeIsQuiz(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := LoadBaseline(configPath, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := baseline.Factory.VerifyMode; got.Value != ModeQuiz || got.Source != SourceFactory {
		t.Fatalf("factory verify mode = %+v, want factory quiz", got)
	}
}

func TestLoadConfigRejectsExplicitEmptyQuizBanks(t *testing.T) {
	for _, config := range []map[string]any{
		{"verify_mode": ModeQuiz, "questions": []any{}},
		{"group_ids": []int64{-1009000003001}, "verify_mode": ModeQuiz, "questions": []any{}},
	} {
		if _, err := LoadConfig(writeConfig(t, config)); err == nil {
			t.Errorf("explicitly empty quiz bank was accepted: %#v", config)
		}
	}
}
