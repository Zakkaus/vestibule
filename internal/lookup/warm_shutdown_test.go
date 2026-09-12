package lookup

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// A warm-up outliving its service raced the next service's configuration in the
// app tests (CI run 34713781235). Shutdown must end it before returning.
func TestShutdownWaitsForDemandedWarmUp(t *testing.T) {
	service := New(nil, nil, &settings.Config{}, "")
	service.DemandWarm()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	service.Shutdown(ctx)
	if ctx.Err() != nil {
		t.Fatalf("shutdown waited for the deadline instead of the warm-up")
	}
	select {
	case <-service.warmDone:
	default:
		t.Fatalf("warm-up still running after Shutdown")
	}
}

func TestShutdownWithoutWarmUpReturnsAtOnce(t *testing.T) {
	service := New(nil, nil, &settings.Config{}, "")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	service.Shutdown(ctx)
	if ctx.Err() != nil {
		t.Fatalf("shutdown blocked with no warm-up demanded")
	}
	service.DemandWarm()
	select {
	case <-service.warmDone:
	case <-time.After(time.Second):
		t.Fatalf("a warm-up demanded after Shutdown must not start")
	}
}
