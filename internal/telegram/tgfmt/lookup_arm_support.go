package tgfmt

import (
	"html"
	"strings"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
)

var armMessages = map[lookup.ArmState]i18n.Text{
	lookup.ArmQueryFailed:         i18n.Messages.LookupDistros.Armpkgs.QueryFailed,
	lookup.ArmNotInOfficialTree:   i18n.Messages.LookupDistros.Armpkgs.NotInOfficialTree,
	lookup.ArmNoPackage:           i18n.Messages.LookupDistros.Armpkgs.NoArm64Package,
	lookup.ArmNotInFedora:         i18n.Messages.LookupDistros.Armpkgs.NotInFedora,
	lookup.ArmFedoraFailed:        i18n.Messages.LookupDistros.Armpkgs.FedoraQueryFailed,
	lookup.ArmPKGBUILDParseFailed: i18n.Messages.LookupDistros.Armpkgs.PKGBUILDParseFailed,
	lookup.ArmAny:                 i18n.Messages.LookupDistros.Armpkgs.AnyArchitecture,
	lookup.ArmAarch64:             i18n.Messages.LookupDistros.Armpkgs.DeclaresAarch64,
	lookup.Arm32Only:              i18n.Messages.LookupDistros.Armpkgs.Arm32Only,
	lookup.ArmX86Only:             i18n.Messages.LookupDistros.Armpkgs.X86Only,
	lookup.ArmNotInAUR:            i18n.Messages.LookupDistros.Armpkgs.NotInAUR,
	lookup.ArmAURFailed:           i18n.Messages.LookupDistros.Armpkgs.AURQueryFailed,
	lookup.ArmNotPackaged:         i18n.Messages.LookupDistros.Armpkgs.NotPackaged,
	lookup.ArmPackaged:            i18n.Messages.LookupDistros.Armpkgs.Packaged,
}

func RenderArmSupport(l i18n.Lang, result lookup.ArmSupport) string {
	messages := i18n.Messages.LookupDistros.Armpkgs
	switch result.State {
	case lookup.ArmKeywords:
		switch {
		case result.Stable != "" && result.Testing != "":
			return messages.StableTesting.Render(l, result.Stable, result.Testing)
		case result.Stable != "":
			return messages.StableOnly.Render(l, result.Stable)
		case result.Testing != "":
			return messages.TestingOnly.Render(l, result.Testing)
		default:
			return messages.NoArm64Keyword.For(l)
		}
	case lookup.ArmAvailable:
		suite := result.Suite
		if result.Development {
			suite = messages.DevelopmentSuite.Render(l, suite)
		}
		return messages.Available.Render(l, suite, result.Version)
	case lookup.ArmFedoraAvailable:
		return messages.FedoraRawhide.Render(l, result.Version)
	default:
		return armMessages[result.State].For(l)
	}
}

func RenderArm(l i18n.Lang, name string, result lookup.ArmLookup) (string, bool) {
	messages := i18n.Messages.LookupPackages.Arm
	if !result.Available {
		return messages.OfficialUnavailable.For(l), false
	}
	if !result.Found {
		return messages.NotFound.Render(l, name), false
	}
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(messages.Heading.Render(l, esc(result.URL), esc(result.Atom)))
	switch {
	case !result.KeywordsAvailable:
		b.WriteString("\n" + messages.KeywordUnavailable.For(l))
	case result.Stable != "" && result.Testing != "" && result.Stable != result.Testing:
		b.WriteString(messages.StableTesting.Render(l, esc(result.Stable), esc(result.Testing)))
	case result.Stable != "":
		b.WriteString(messages.StableOnly.Render(l, esc(result.Stable)))
	case result.Testing != "":
		b.WriteString(messages.TestingOnly.Render(l, esc(result.Testing)))
	default:
		b.WriteString("\n" + messages.NoKeyword.For(l))
	}
	return b.String(), true
}

func RenderArmPackages(l i18n.Lang, name string, results []lookup.ArmRow) string {
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(i18n.Messages.LookupDistros.Armpkgs.Heading.Render(l, esc(name)))
	for _, r := range results {
		b.WriteString(i18n.Messages.LookupDistros.Armpkgs.Row.Render(l, esc(r.URL), esc(r.Label), esc(RenderArmSupport(l, r.Support))))
	}
	b.WriteByte('\n')
	b.WriteString(i18n.Messages.LookupDistros.Armpkgs.Footer.For(l))
	return b.String()
}
