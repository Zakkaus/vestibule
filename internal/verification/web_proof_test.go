package verification

import (
	"strconv"
	"testing"
)

func TestPoWBitBoundaryAndNonceDomain(t *testing.T) {
	var digest [32]byte
	digest[1] = 0x0f
	if !leadingZeroBits(digest, 12) {
		t.Fatal("12 zero bits rejected at partial-byte boundary")
	}
	digest[1] = 0x1f
	if leadingZeroBits(digest, 12) {
		t.Fatal("11 zero bits accepted as 12")
	}
	var salt [16]byte
	nonce := ""
	for n := range uint64(1 << 20) {
		candidate := strconv.FormatUint(n, 10)
		if VerifyPoW(salt, candidate, 12) {
			nonce = candidate
			break
		}
	}
	if nonce == "" {
		t.Fatal("known salt has no proof in fixture range")
	}
	salt[0] = 1
	if VerifyPoW(salt, nonce, 12) {
		t.Fatal("proof accepted against a different salt")
	}
	for _, malformed := range []string{"", "-1", "+1", " 1", "1.0", "18446744073709551616", "000000000000000000000"} {
		if VerifyPoW(salt, malformed, 12) {
			t.Fatalf("malformed nonce %q accepted", malformed)
		}
	}
	if VerifyPoW(salt, nonce, 11) || VerifyPoW(salt, nonce, 23) {
		t.Fatal("out-of-domain difficulty accepted")
	}
}
