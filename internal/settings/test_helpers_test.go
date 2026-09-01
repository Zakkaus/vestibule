package settings

import (
	"errors"
	"reflect"
	"testing"
)

func requireEqual[T comparable](t *testing.T, got, want T, label string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func requireDeepEqual(t *testing.T, got, want any, label string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func requireErrorIs(t *testing.T, err, target error, label string) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s = %v, want %v", label, err, target)
	}
}

func requireSettingsView(t *testing.T, store *Store, groupID int64) GroupView {
	t.Helper()
	group, ok := store.Settings(groupID)
	if !ok {
		t.Fatalf("settings group %d is absent", groupID)
	}
	return group
}

func requirePointerValue[T comparable](t *testing.T, got *T, want T, label string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil", label)
	}
	requireEqual(t, *got, want, label)
}

func requirePointerDeepEqual[T any](t *testing.T, got *T, want T, label string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s is nil", label)
	}
	requireDeepEqual(t, *got, want, label)
}
