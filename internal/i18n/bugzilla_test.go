package i18n

import "testing"

func TestTranslateBugValueLocalizesKnownEnumsAndPreservesUnknownValues(t *testing.T) {
	tests := []struct {
		name, value string
		text        Text
	}{
		{name: "status UNCONFIRMED", value: "UNCONFIRMED", text: Messages.LookupContent.Bug.Status.Unconfirmed},
		{name: "status CONFIRMED", value: "CONFIRMED", text: Messages.LookupContent.Bug.Status.Confirmed},
		{name: "status IN_PROGRESS", value: "IN_PROGRESS", text: Messages.LookupContent.Bug.Status.InProgress},
		{name: "status RESOLVED", value: "RESOLVED", text: Messages.LookupContent.Bug.Status.Resolved},
		{name: "status VERIFIED", value: "VERIFIED", text: Messages.LookupContent.Bug.Status.Verified},
		{name: "resolution FIXED", value: "FIXED", text: Messages.LookupContent.Bug.Resolution.Fixed},
		{name: "resolution WONTFIX", value: "WONTFIX", text: Messages.LookupContent.Bug.Resolution.WontFix},
		{name: "resolution CANTFIX", value: "CANTFIX", text: Messages.LookupContent.Bug.Resolution.CantFix},
		{name: "resolution DUPLICATE", value: "DUPLICATE", text: Messages.LookupContent.Bug.Resolution.Duplicate},
		{name: "resolution INVALID", value: "INVALID", text: Messages.LookupContent.Bug.Resolution.Invalid},
		{name: "resolution WORKSFORME", value: "WORKSFORME", text: Messages.LookupContent.Bug.Resolution.WorksForMe},
		{name: "resolution OBSOLETE", value: "OBSOLETE", text: Messages.LookupContent.Bug.Resolution.Obsolete},
		{name: "resolution UPSTREAM", value: "UPSTREAM", text: Messages.LookupContent.Bug.Resolution.Upstream},
		{name: "resolution PKGREMOVED", value: "PKGREMOVED", text: Messages.LookupContent.Bug.Resolution.PackageRemoved},
		{name: "resolution NEEDINFO", value: "NEEDINFO", text: Messages.LookupContent.Bug.Resolution.NeedInfo},
		{name: "resolution TEST-REQUEST", value: "TEST-REQUEST", text: Messages.LookupContent.Bug.Resolution.TestRequest},
		{name: "resolution PENDING-UPSTREAM", value: "PENDING-UPSTREAM", text: Messages.LookupContent.Bug.Resolution.PendingUpstream},
		{name: "severity blocker", value: "blocker", text: Messages.LookupContent.Bug.Severity.Blocker},
		{name: "severity critical", value: "critical", text: Messages.LookupContent.Bug.Severity.Critical},
		{name: "severity major", value: "major", text: Messages.LookupContent.Bug.Severity.Major},
		{name: "severity normal", value: "normal", text: Messages.LookupContent.Bug.Severity.Normal},
		{name: "severity minor", value: "minor", text: Messages.LookupContent.Bug.Severity.Minor},
		{name: "severity trivial", value: "trivial", text: Messages.LookupContent.Bug.Severity.Trivial},
		{name: "severity enhancement", value: "enhancement", text: Messages.LookupContent.Bug.Severity.Enhancement},
		{name: "priority Highest", value: "Highest", text: Messages.LookupContent.Bug.Priority.Highest},
		{name: "priority High", value: "High", text: Messages.LookupContent.Bug.Priority.High},
		{name: "priority Normal", value: "Normal", text: Messages.LookupContent.Bug.Priority.Normal},
		{name: "priority Low", value: "Low", text: Messages.LookupContent.Bug.Priority.Low},
		{name: "priority Lowest", value: "Lowest", text: Messages.LookupContent.Bug.Priority.Lowest},
	}

	for _, language := range Languages() {
		t.Run(language.String(), func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					want := tt.text.For(language)
					if got := TranslateBugValue(language, tt.value); got != want {
						t.Errorf("/bug displayed raw or wrong %s as %q, want localized label %q", tt.name, got, want)
					}
				})
			}
			const unknown = "FUTURE_UPSTREAM_ENUM"
			if got := TranslateBugValue(language, unknown); got != unknown {
				t.Errorf("/bug rewrote unknown upstream enum %q as %q; unknown values must remain visible", unknown, got)
			}
		})
	}
}
