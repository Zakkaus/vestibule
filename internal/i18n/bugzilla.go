package i18n

// Only Bugzilla's finite enum values are localized; official identifiers stay unchanged.
var (
	bugStatusMessages = map[string]Text{
		"UNCONFIRMED": Messages.LookupContent.Bug.Status.Unconfirmed,
		"CONFIRMED":   Messages.LookupContent.Bug.Status.Confirmed,
		"IN_PROGRESS": Messages.LookupContent.Bug.Status.InProgress,
		"RESOLVED":    Messages.LookupContent.Bug.Status.Resolved,
		"VERIFIED":    Messages.LookupContent.Bug.Status.Verified,
	}
	bugResolutionMessages = map[string]Text{
		"FIXED":            Messages.LookupContent.Bug.Resolution.Fixed,
		"WONTFIX":          Messages.LookupContent.Bug.Resolution.WontFix,
		"CANTFIX":          Messages.LookupContent.Bug.Resolution.CantFix,
		"DUPLICATE":        Messages.LookupContent.Bug.Resolution.Duplicate,
		"INVALID":          Messages.LookupContent.Bug.Resolution.Invalid,
		"WORKSFORME":       Messages.LookupContent.Bug.Resolution.WorksForMe,
		"OBSOLETE":         Messages.LookupContent.Bug.Resolution.Obsolete,
		"UPSTREAM":         Messages.LookupContent.Bug.Resolution.Upstream,
		"PKGREMOVED":       Messages.LookupContent.Bug.Resolution.PackageRemoved,
		"NEEDINFO":         Messages.LookupContent.Bug.Resolution.NeedInfo,
		"TEST-REQUEST":     Messages.LookupContent.Bug.Resolution.TestRequest,
		"PENDING-UPSTREAM": Messages.LookupContent.Bug.Resolution.PendingUpstream,
	}
	bugSeverityMessages = map[string]Text{
		"blocker":     Messages.LookupContent.Bug.Severity.Blocker,
		"critical":    Messages.LookupContent.Bug.Severity.Critical,
		"major":       Messages.LookupContent.Bug.Severity.Major,
		"normal":      Messages.LookupContent.Bug.Severity.Normal,
		"minor":       Messages.LookupContent.Bug.Severity.Minor,
		"trivial":     Messages.LookupContent.Bug.Severity.Trivial,
		"enhancement": Messages.LookupContent.Bug.Severity.Enhancement,
	}
	bugPriorityMessages = map[string]Text{
		"Highest": Messages.LookupContent.Bug.Priority.Highest,
		"High":    Messages.LookupContent.Bug.Priority.High,
		"Normal":  Messages.LookupContent.Bug.Priority.Normal,
		"Low":     Messages.LookupContent.Bug.Priority.Low,
		"Lowest":  Messages.LookupContent.Bug.Priority.Lowest,
	}
	bugStatusZH     = localizedBugLabels(LangZH, bugStatusMessages)
	bugResolutionZH = localizedBugLabels(LangZH, bugResolutionMessages)
	bugSeverityZH   = localizedBugLabels(LangZH, bugSeverityMessages)
	bugPriorityZH   = localizedBugLabels(LangZH, bugPriorityMessages)
)

func localizedBugLabels(l Lang, messages map[string]Text) map[string]string {
	labels := make(map[string]string, len(messages))
	for code, message := range messages {
		labels[code] = message.For(l)
	}
	return labels
}

// TranslateBugValue localizes known Bugzilla enums and preserves unknown values.
func TranslateBugValue(l Lang, value string) string {
	if l == LangZH {
		for _, labels := range [...]map[string]string{bugStatusZH, bugResolutionZH, bugSeverityZH, bugPriorityZH} {
			if translated, ok := labels[value]; ok {
				return translated
			}
		}
		return value
	}
	for _, messages := range [...]map[string]Text{bugStatusMessages, bugResolutionMessages, bugSeverityMessages, bugPriorityMessages} {
		if message, ok := messages[value]; ok {
			return message.For(l)
		}
	}
	return value
}
