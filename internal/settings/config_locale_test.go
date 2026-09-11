package settings

import "testing"

func TestLoadConfigJapaneseAndRussianAtEveryLayer(t *testing.T) {
	base := func() map[string]any {
		return map[string]any{
			"group_ids":   []int{-100},
			"verify_mode": ModeKernel,
		}
	}
	for _, language := range []string{"ja", "ru"} {
		for layer, mutate := range map[string]func(map[string]any){
			"global": func(value map[string]any) { value["lang"] = language },
			"group": func(value map[string]any) {
				value["groups"] = []map[string]any{{"id": -100, "lang": language}}
				value["group_ids"] = nil
			},
			"feed": func(value map[string]any) {
				value["feeds"] = []map[string]any{{"chat_id": -300, "lang": language}}
			},
		} {
			t.Run(language+"/"+layer, func(t *testing.T) {
				value := base()
				mutate(value)
				loaded, err := LoadConfig(writeConfig(t, value))
				if err != nil {
					t.Fatalf("%s language was rejected: %v", language, err)
				}
				switch layer {
				case "global":
					if loaded.Lang != language {
						t.Fatalf("global language = %q, want %q", loaded.Lang, language)
					}
				case "group":
					if got := loaded.LangForGroup(-100); got != language {
						t.Fatalf("group language = %q, want %q", got, language)
					}
				case "feed":
					if got := loaded.Feeds[0].Lang; got != language {
						t.Fatalf("feed language = %q, want %q", got, language)
					}
				}
			})
		}
	}
}
