package i18n

// FeedCatalog contains feed publication text.
type FeedCatalog struct {
	// Bug contains Bugzilla feed field labels and separators.
	Bug FeedBugCatalog
	// GitHub contains GitHub feed message templates.
	GitHub FeedGitHubCatalog
	// Config contains user-facing configuration policy refusals.
	Config FeedConfigCatalog
}

// FeedGitHubCatalog contains localized GitHub feed message templates.
type FeedGitHubCatalog struct {
	// Commit is the complete commit message template.
	Commit Format
	// Branch labels an explicitly configured branch.
	Branch Format
	// Author labels a commit author when present.
	Author Format
	// IssueOpened is the complete issue creation message template.
	IssueOpened Format
	// PullOpened is the complete pull-request creation message template.
	PullOpened Format
}

// FeedBugCatalog contains Bugzilla feed field labels and separators.
type FeedBugCatalog struct {
	// FieldSeparator separates a field label from its value.
	FieldSeparator Text
	// StatusResolutionSeparator separates a bug status from its resolution.
	StatusResolutionSeparator Text
	// Status labels the bug status.
	Status Text
	// ProductComponent labels the Bugzilla product and component.
	ProductComponent Text
	// Priority labels the bug priority.
	Priority Text
	// Severity labels the bug severity.
	Severity Text
	// Keywords labels Bugzilla keywords.
	Keywords Text
	// Packages labels affected packages.
	Packages Text
	// Assignee labels the bug assignee.
	Assignee Text
	// Reporter labels the bug reporter.
	Reporter Text
	// CreationDate labels the bug creation date.
	CreationDate Text
}

// FeedConfigCatalog contains user-facing configuration policy refusals.
type FeedConfigCatalog struct {
}
