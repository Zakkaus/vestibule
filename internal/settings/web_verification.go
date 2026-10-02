package settings

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	CaptchaFallback = "fallback"
	CaptchaApprove  = "approve"
	CaptchaDecline  = "decline"
)

// ErrWebVerificationSettings identifies an invalid web-proof value or unavailable delivery capability.
var ErrWebVerificationSettings = errors.New("invalid web verification settings")

// WebCapabilities is supplied by process assembly; provider credentials never enter settings.
type WebCapabilities struct {
	ConsoleURL         string
	TurnstileAvailable bool
}

func IsWebMode(mode string) bool { return mode == ModePoW || mode == ModeCaptcha }

func validateEffectiveWebProof(group *effectiveGroup) error {
	if group.powBits.Value < 12 || group.powBits.Value > 22 {
		return fmt.Errorf("%w: pow_bits must be between 12 and 22", ErrWebVerificationSettings)
	}
	switch group.captchaUnavailable.Value {
	case CaptchaFallback, CaptchaApprove, CaptchaDecline:
	default:
		return fmt.Errorf("%w: invalid captcha_unavailable %q", ErrWebVerificationSettings, group.captchaUnavailable.Value)
	}
	return nil
}

func validateConfigWebProof(bits *int, policy *string) error {
	group := effectiveGroup{powBits: Setting[int]{Value: embeddedDefaults.Factory.PoWBits},
		captchaUnavailable: Setting[string]{Value: embeddedDefaults.Factory.CaptchaUnavailable}}
	if bits != nil {
		group.powBits.Value = *bits
	}
	if policy != nil {
		group.captchaUnavailable.Value = *policy
	}
	return validateEffectiveWebProof(&group)
}

// SetWebCapabilities updates runtime availability without changing stored settings or revisions.
func (s *Store) SetWebCapabilities(capabilities WebCapabilities) {
	s.writer.Lock()
	defer s.writer.Unlock()
	s.webCapabilities = capabilities
}

// WebModeUnavailable returns an adapter-neutral reason, or empty when the mode is available.
func (s *Store) WebModeUnavailable(mode string) string {
	s.writer.Lock()
	defer s.writer.Unlock()
	return s.webCapabilities.unavailable(mode)
}

func (c WebCapabilities) unavailable(mode string) string {
	if !IsWebMode(mode) {
		return ""
	}
	u, err := url.Parse(c.ConsoleURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return "https_required"
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()) {
		return "loopback"
	}
	if mode == ModeCaptcha && !c.TurnstileAvailable {
		return "turnstile_missing"
	}
	return ""
}

func (s *Store) validateWebSettings(group *effectiveGroup) error {
	if IsWebMode(group.verifyMode.Value) && group.deliveryMode.Value == DeliveryGroup {
		return fmt.Errorf("%w: web verification requires private delivery", ErrWebVerificationSettings)
	}
	if reason := s.webCapabilities.unavailable(group.verifyMode.Value); reason != "" {
		return fmt.Errorf("%w: web verification unavailable: %s", ErrWebVerificationSettings, reason)
	}
	return nil
}
