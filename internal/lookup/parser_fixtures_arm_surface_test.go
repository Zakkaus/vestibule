package lookup_test

import (
	"context"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"

	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
)

func TestReleaseMetadataLiveFixtures(t *testing.T) {
	capturedAt := time.Date(2026, time.August, 25, 0, 0, 0, 0, time.UTC)
	loadDebianReleaseFixture(t, capturedAt)
	loadUbuntuReleaseFixture(t, capturedAt)

	debianMadison := lookup.ParseMadisonFixture(string(lookup.TestUpstreamFixture(t, "debian-madison-vim.txt")))
	if len(debianMadison) != 5 || debianMadison[0].Suite != "bullseye" ||
		debianMadison[len(debianMadison)-1].Suite != "sid" {
		t.Fatalf("Debian Madison decoded incorrectly: %+v", debianMadison)
	}
	suite, version, dev := lookup.PickMadisonFixture(debianMadison, lookup.DebianDevSuite)
	if suite != "trixie" || version != "2:9.1.1230-2" || dev {
		t.Fatalf("Debian transition selection = suite %q version %q dev %v", suite, version, dev)
	}
	const debianMadisonBase = "https://qa.debian.org/madison.php?package="
	wantDebian := i18n.Messages.LookupDistros.Armpkgs.Available.Render(i18n.LangEN, "trixie", "2:9.1.1230-2")
	lookup.TestWithFixtureHTTP(t, map[string]string{
		debianMadisonBase + "vim&text=on&a=arm64": "debian-madison-vim.txt",
	}, func() {
		if got := tgfmt.RenderArmSupport(i18n.LangEN, lookup.MadisonArmStatus(context.Background(), debianMadisonBase, "vim", lookup.DebianDevSuite)); got != wantDebian {
			t.Fatalf("Debian Madison status = %q, want %q", got, wantDebian)
		}
	})
	ubuntuMadison := lookup.ParseMadisonFixture(string(lookup.TestUpstreamFixture(t, "ubuntu-madison-vim.txt")))
	suite, version, dev = lookup.PickMadisonFixture(ubuntuMadison, func(s string) bool { return s == "stonking" })
	if suite != "resolute" || version != "2:9.1.2141-1ubuntu4" || dev {
		t.Fatalf("Ubuntu transition selection = suite %q version %q dev %v", suite, version, dev)
	}
	const ubuntuMadisonBase = "https://people.canonical.com/~ubuntu-archive/madison.cgi?package="
	wantUbuntu := i18n.Messages.LookupDistros.Armpkgs.Available.Render(i18n.LangEN, "resolute", "2:9.1.2141-1ubuntu4")
	lookup.TestWithFixtureHTTP(t, map[string]string{
		ubuntuMadisonBase + "vim&text=on&a=arm64": "ubuntu-madison-vim.txt",
	}, func() {
		if got := tgfmt.RenderArmSupport(i18n.LangEN, lookup.MadisonArmStatus(context.Background(), ubuntuMadisonBase, "vim", lookup.UbuntuDevSuite)); got != wantUbuntu {
			t.Fatalf("Ubuntu Madison status = %q, want %q", got, wantUbuntu)
		}
	})
}

func TestFedoraArmLiveFixture(t *testing.T) {
	want := i18n.Messages.LookupDistros.Armpkgs.FedoraQueryFailed.For(i18n.LangEN)
	const fedoraURL = "https://mdapi.fedoraproject.org/rawhide/pkg/curl"
	lookup.TestWithFixtureHTTP(t, map[string]string{fedoraURL: "fedora-curl.json"}, func() {
		if got := tgfmt.RenderArmSupport(i18n.LangEN, lookup.FedoraArmStatus(context.Background(), "curl")); got != want {
			t.Fatalf("x86_64 Fedora metadata must not prove arm64 support: got %q want %q", got, want)
		}
	})
}

func TestAURArmLiveFixture(t *testing.T) {
	want := i18n.Messages.LookupDistros.Armpkgs.DeclaresAarch64.For(i18n.LangEN)
	const aurURL = "https://aur.archlinux.org/cgit/aur.git/plain/PKGBUILD?h=yay"
	lookup.TestWithFixtureHTTP(t, map[string]string{aurURL: "aur-yay.PKGBUILD"}, func() {
		if got := tgfmt.RenderArmSupport(i18n.LangEN, lookup.AurArmStatus(context.Background(), "yay")); got != want {
			t.Fatalf("AUR PKGBUILD architecture = %q, want %q", got, want)
		}
	})
}

func TestArchLinuxARMLiveFixture(t *testing.T) {
	want := i18n.Messages.LookupDistros.Armpkgs.Packaged.For(i18n.LangEN)
	const alarmURL = "https://archlinuxarm.org/packages/aarch64/curl"
	lookup.TestWithFixtureHTTP(t, map[string]string{alarmURL: "alarm-curl.html"}, func() {
		if got := tgfmt.RenderArmSupport(i18n.LangEN, lookup.AlarmArmStatus(context.Background(), "curl")); got != want {
			t.Fatalf("Arch Linux ARM package page = %q, want %q", got, want)
		}
	})
}

func loadDebianReleaseFixture(t *testing.T, capturedAt time.Time) {
	t.Helper()
	const debianURL = "https://debian.pages.debian.net/distro-info-data/debian.csv"
	var debianData lookup.TestDebianReleaseData
	lookup.TestWithFixtureHTTP(t, map[string]string{debianURL: "debian.csv"}, func() {
		debianData = lookup.TestFetchDebianStatus(context.Background(), capturedAt)
	})
	debian := debianData.Roles
	if debian["13"] != "stable" || debian["12"] != "oldstable" ||
		debian["11"] != "oldoldstable" || debian["14"] != "testing" {
		t.Fatalf("Debian release roles decoded incorrectly: %v", debian)
	}
	(*lookup.TestRelInfo).Mu.Lock()
	oldDebianSeries := (*lookup.TestRelInfo).DebianSer
	(*lookup.TestRelInfo).DebianSer = debianData.Series
	(*lookup.TestRelInfo).Mu.Unlock()
	t.Cleanup(func() {
		(*lookup.TestRelInfo).Mu.Lock()
		(*lookup.TestRelInfo).DebianSer = oldDebianSeries
		(*lookup.TestRelInfo).Mu.Unlock()
	})
}

func loadUbuntuReleaseFixture(t *testing.T, capturedAt time.Time) {
	t.Helper()
	const ubuntuURL = "https://debian.pages.debian.net/distro-info-data/ubuntu.csv"
	var ubuntuSeries map[string]bool
	lookup.TestWithFixtureHTTP(t, map[string]string{ubuntuURL: "ubuntu.csv"}, func() {
		lts, released, eol, series := lookup.TestFetchUbuntu(context.Background(), capturedAt)
		if !lts["26.04"] || !released["26.04"] || released["26.10"] ||
			!eol["20.04"] || series["stonking"] {
			t.Fatalf("Ubuntu release metadata decoded incorrectly: lts=%v released=%v eol=%v series=%v",
				lts, released, eol, series)
		}
		ubuntuSeries = series
	})
	(*lookup.TestRelInfo).Mu.Lock()
	oldUbuntuSeries := (*lookup.TestRelInfo).UbuntuSer
	(*lookup.TestRelInfo).UbuntuSer = ubuntuSeries
	(*lookup.TestRelInfo).Mu.Unlock()
	t.Cleanup(func() {
		(*lookup.TestRelInfo).Mu.Lock()
		(*lookup.TestRelInfo).UbuntuSer = oldUbuntuSeries
		(*lookup.TestRelInfo).Mu.Unlock()
	})
}
