package i18n

import (
	"strings"
	"testing"
)

// Every locale shows the distribution-neutral shape accepted by the kernel rule.
func TestKernelPromptUsesCanonicalExample(t *testing.T) {
	for _, l := range []Lang{LangEN, LangZH, LangZHHant} {
		for name, prompt := range map[string]string{
			"kernel_prompt":      Messages.Verification.Challenge.KernelPrompt.Render(l, "Q", 3),
			"kernel_prompt_held": Messages.Verification.Challenge.KernelPromptHeld.Render(l, "Q", 3),
		} {
			want := "X.Y.Z"
			if !strings.Contains(prompt, want) {
				t.Errorf("%v %s: format example is not %q: %q", l, name, want, prompt)
			}
			if strings.Contains(prompt, "{ks}") {
				t.Errorf("%v %s: removed edition token remains in prompt", l, name)
			}
		}
	}
}
