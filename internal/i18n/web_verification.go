package i18n

// VerificationWebCatalog serves the applicant page and its private delivery button.
type VerificationWebCatalog struct {
	// Title names the public verification page.
	Title Text
	// PoWIntro explains local proof computation.
	PoWIntro Text
	// CaptchaIntro explains the human verification step.
	CaptchaIntro Text
	// Computing describes the active proof search.
	Computing Text
	// Ready describes a proof ready to submit.
	Ready Text
	// Progress labels the number of hashes checked.
	Progress Text
	// Start labels the computation button.
	Start Text
	// Submit labels the ordinary proof form submission.
	Submit Text
	// Received acknowledges receipt without promising Telegram settlement.
	Received Text
	// Settled is shared by every unavailable token response.
	Settled Text
	// Wrong reports a charged failed attempt.
	Wrong Text
	// ChannelRequired directs the applicant to the Telegram channel guidance.
	ChannelRequired Text
	// Fallback directs the applicant to the replacement question.
	Fallback Text
	// Unavailable reports a temporary request failure.
	Unavailable Text
	// JavaScriptRequired names the browser prerequisite.
	JavaScriptRequired Text
	// Privacy discloses Turnstile browser data sharing.
	Privacy Text
	// DMPrompt explains the private URL button.
	DMPrompt Text
	// Button labels the private URL button.
	Button Text
	// OperatorAlert reports a provider configuration fault without proofs or secrets.
	OperatorAlert Format
}
