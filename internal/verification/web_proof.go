package verification

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// WebTokenRecord contains only challenge-subordinate proof material.
type WebTokenRecord struct {
	ChallengeID string
	TokenHash   string
	Salt        [16]byte
	PoWBits     int
	IssuedAt    int64
}

// WebStore requires durable, conditional token writes and in-place fallback.
type WebStore interface {
	IssueWebToken(context.Context, PendingRef, WebTokenRecord) (bool, error)
	LoadWebToken(context.Context, string, string, int64) (WebTokenRecord, bool, error)
	ResolveWebToken(context.Context, string, int64) (WebTokenRecord, bool, error)
	FallbackWeb(context.Context, PendingRef, PendingRecord, *WebClaim) (bool, error)
}

type WebClaim struct {
	TokenHash string
	Now       int64
}

type WebProof struct {
	Token           string
	Nonce           string
	CaptchaResponse string
}

type WebAnswer string

const (
	WebReceived        WebAnswer = "received"
	WebSettled         WebAnswer = "settled"
	WebWrong           WebAnswer = "wrong"
	WebChannelRequired WebAnswer = "channel_required"
	WebFallback        WebAnswer = "fallback"
)

type WebResult struct {
	Outcome       WebAnswer
	OperatorAlert bool
}

func ChallengeID(ref PendingRef) string {
	return strconv.FormatInt(ref.GroupID, 10) + ":" + strconv.FormatInt(ref.UserID, 10) + ":" + ref.Nonce
}
func HashWebToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func NewWebToken(bits int, issuedAt int64) (string, WebTokenRecord, error) {
	var token [16]byte
	record := WebTokenRecord{PoWBits: bits, IssuedAt: issuedAt}
	if _, err := rand.Read(token[:]); err != nil {
		return "", record, err
	}
	if _, err := rand.Read(record.Salt[:]); err != nil {
		return "", record, err
	}
	raw := hex.EncodeToString(token[:])
	record.TokenHash = HashWebToken(raw)
	return raw, record, nil
}

// VerifyPoW hashes salt bytes followed by the decimal uint64 nonce, not its numeric value.
func VerifyPoW(salt [16]byte, nonce string, bits int) bool {
	if len(nonce) == 0 || len(nonce) > 20 || bits < 12 || bits > 22 {
		return false
	}
	for i := range nonce {
		if nonce[i] < '0' || nonce[i] > '9' {
			return false
		}
	}
	if _, err := strconv.ParseUint(nonce, 10, 64); err != nil {
		return false
	}
	var input [36]byte
	copy(input[:16], salt[:])
	copy(input[16:], nonce)
	sum := sha256.Sum256(input[:16+len(nonce)])
	return leadingZeroBits(sum, bits)
}

func leadingZeroBits(sum [32]byte, bits int) bool {
	for _, b := range sum[:bits/8] {
		if b != 0 {
			return false
		}
	}
	remainder := uint(bits % 8)
	return remainder == 0 || sum[bits/8]>>(8-remainder) == 0
}
