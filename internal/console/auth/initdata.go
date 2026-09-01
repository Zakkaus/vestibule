// Package auth validates Telegram Mini App identity and manages console credentials.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidInitData = errors.New("invalid Telegram init data")
	ErrInitDataExpired = errors.New("expired Telegram init data")
)

// TelegramIdentity is the authenticated Telegram account carried by initData.
type TelegramIdentity struct {
	ID       int64
	AuthDate time.Time
}

// VerifyInitData verifies the Telegram Mini App signature and freshness without retaining raw
// initData. Its first HMAC uses "WebAppData" as the key and the bot token as the message.
func VerifyInitData(raw, botToken string, now time.Time, maxAge time.Duration) (TelegramIdentity, error) {
	values, err := parseInitData(raw)
	if err != nil {
		return TelegramIdentity{}, err
	}
	if !validInitDataSignature(values, botToken) {
		return TelegramIdentity{}, ErrInvalidInitData
	}
	identity, err := initDataIdentity(values)
	if err != nil {
		return TelegramIdentity{}, err
	}
	if identity.AuthDate.After(now) || !identity.AuthDate.Add(maxAge).After(now) {
		return TelegramIdentity{}, ErrInitDataExpired
	}
	return identity, nil
}

func parseInitData(raw string) (url.Values, error) {
	if raw == "" || len(raw) > 8192 {
		return nil, ErrInvalidInitData
	}
	values, err := url.ParseQuery(raw)
	if err != nil || len(values["hash"]) != 1 || values.Get("hash") == "" {
		return nil, ErrInvalidInitData
	}
	for key, value := range values {
		if key == "" || len(value) != 1 {
			return nil, ErrInvalidInitData
		}
	}
	return values, nil
}

func validInitDataSignature(values url.Values, botToken string) bool {
	if botToken == "" {
		return false
	}
	received, err := hex.DecodeString(values.Get("hash"))
	if err != nil || len(received) != sha256.Size {
		return false
	}
	keys := make([]string, 0, len(values)-1)
	for key := range values {
		if key != "hash" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	secret := hmac.New(sha256.New, []byte("WebAppData"))
	_, _ = secret.Write([]byte(botToken))
	check := hmac.New(sha256.New, secret.Sum(nil))
	_, _ = check.Write([]byte(strings.Join(parts, "\n")))
	return hmac.Equal(received, check.Sum(nil))
}

func initDataIdentity(values url.Values) (TelegramIdentity, error) {
	authDate, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil || authDate <= 0 {
		return TelegramIdentity{}, ErrInvalidInitData
	}
	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil || user.ID <= 0 {
		return TelegramIdentity{}, ErrInvalidInitData
	}
	return TelegramIdentity{ID: user.ID, AuthDate: time.Unix(authDate, 0)}, nil
}
