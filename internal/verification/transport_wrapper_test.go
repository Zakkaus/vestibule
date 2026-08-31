package verification

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/config"
)

// Recovery may receive a wrapper around the gateway used by handlers. The core must keep the
// interface value intact instead of asserting a concrete transport type.
type wrappedGateway struct {
	Gateway
}

func TestTransportAcceptsAWrappedGateway(t *testing.T) {
	v := newTestService(&config.Config{GroupIDs: []int64{-100}, Lang: "en"})
	base := newFakeVerifyBot()
	wrapped := wrappedGateway{Gateway: base}
	if got := v.gatewayFor(wrapped); got != wrapped {
		t.Errorf("gatewayFor changed the supplied wrapper: got=%T want=%T", got, wrapped)
	}
}

// A gateway whose implementation is supplied later must not trigger a concrete-type assertion.
type opaqueGateway struct {
	Gateway
}

func TestTransportDoesNotPanicOnAnOpaqueGateway(t *testing.T) {
	v := newTestService(&config.Config{GroupIDs: []int64{-100}, Lang: "en"})
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("resolving a transport panicked: %v", recovered)
		}
	}()
	opaque := opaqueGateway{}
	if got := v.gatewayFor(opaque); got != opaque {
		t.Errorf("gatewayFor changed the opaque gateway: got=%T want=%T", got, opaque)
	}
}
