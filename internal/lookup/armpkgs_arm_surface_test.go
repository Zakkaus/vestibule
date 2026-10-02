package lookup_test

import (
	"context"
	"errors"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestAurArchLabel(t *testing.T) {
	messages := i18n.Messages.LookupDistros.Armpkgs
	for _, c := range []struct{ pkgbuild, want string }{
		{"pkgname=x\narch=('any')\n", messages.AnyArchitecture.For(i18n.LangZH)},
		{"arch=('i686' 'x86_64' 'aarch64' 'armv7h')", messages.DeclaresAarch64.For(i18n.LangZH)},
		{"arch=(x86_64 aarch64)", messages.DeclaresAarch64.For(i18n.LangZH)},
		{"arch=('armv7h' 'armv6h')", messages.Arm32Only.For(i18n.LangZH)},
		{"arch=('x86_64')", messages.X86Only.For(i18n.LangZH)},
		{"pkgname=x\nno arch here", messages.PKGBUILDParseFailed.For(i18n.LangZH)},
	} {
		if got := tgfmt.RenderArmSupport(i18n.LangZH, lookup.TestAurArchLabel(c.pkgbuild)); got != c.want {
			t.Errorf("aurArchLabel(%q) = %q, want %q", c.pkgbuild, got, c.want)
		}
	}
}

func TestFedoraArmStatusAvailability(t *testing.T) {
	messages := i18n.Messages.LookupDistros.Armpkgs
	const foundVersion = "3.4.1-2.fc44"
	for _, tc := range []struct {
		name    string
		version string
		err     error
		want    string
		notWant string
	}{
		{name: "found", version: foundVersion, want: messages.FedoraRawhide.Render(i18n.LangZH, foundVersion)},
		{name: "404", err: &lookup.TestHttpStatusError{Url: "u", Code: 404}, want: messages.NotInFedora.For(i18n.LangZH)},
		{name: "429", err: &lookup.TestHttpStatusError{Url: "u", Code: 429}, want: messages.FedoraQueryFailed.For(i18n.LangZH), notWant: messages.NotInFedora.For(i18n.LangZH)},
		{name: "503", err: &lookup.TestHttpStatusError{Url: "u", Code: 503}, want: messages.FedoraQueryFailed.For(i18n.LangZH), notWant: messages.NotInFedora.For(i18n.LangZH)},
		{name: "network", err: errors.New("connection reset"), want: messages.FedoraQueryFailed.For(i18n.LangZH), notWant: messages.NotInFedora.For(i18n.LangZH)},
		{name: "busy", err: &lookup.TestHttpBusyError{Url: "u", Wait: time.Millisecond}, want: messages.FedoraQueryFailed.For(i18n.LangZH), notWant: messages.NotInFedora.For(i18n.LangZH)},
		{name: "oversized", err: &lookup.TestHttpBodyTooLargeError{Url: "u", Limit: 3}, want: messages.FedoraQueryFailed.For(i18n.LangZH), notWant: messages.NotInFedora.For(i18n.LangZH)},
		{name: "missing version", want: messages.FedoraQueryFailed.For(i18n.LangZH), notWant: messages.NotInFedora.For(i18n.LangZH)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tgfmt.RenderArmSupport(i18n.LangZH, lookup.TestFedoraArmStatusWith(context.Background(), "htop", func(context.Context, string) (string, error) { return tc.version, tc.err }))
			if got != tc.want {
				t.Errorf("fedoraArmStatusWith() = %q, want %q", got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("fedoraArmStatusWith() = %q, unwanted substring %q", got, tc.notWant)
			}
		})
	}
}

func TestGentooArmStatusAvailability(t *testing.T) {
	messages := i18n.Messages.LookupDistros.Armpkgs
	const foundVersion = "3.4.1"
	for _, tc := range []struct {
		name      string
		atoms     []string
		available bool
		want      string
		notWant   string
	}{
		{name: "search unavailable", want: messages.QueryFailed.For(i18n.LangZH), notWant: messages.NotInOfficialTree.For(i18n.LangZH)},
		{name: "answered miss", available: true, want: messages.NotInOfficialTree.For(i18n.LangZH), notWant: messages.QueryFailed.For(i18n.LangZH)},
		{name: "found", atoms: []string{"sys-process/htop"}, available: true, want: messages.StableOnly.Render(i18n.LangZH, foundVersion)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := gentooArmMessageWith(context.Background(), i18n.LangZH, "htop", func(context.Context, string) ([]string, bool) { return tc.atoms, tc.available }, func(context.Context, string) (string, string, bool) { return foundVersion, "", true })
			if got != tc.want {
				t.Errorf("gentooArmStatusWith() = %q, want %q", got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("gentooArmStatusWith() = %q, unwanted substring %q", got, tc.notWant)
			}
		})
	}
}
