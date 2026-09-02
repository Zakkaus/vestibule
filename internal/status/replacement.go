package status

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	replacementUnitFile    = "replacement-unit.env"
	replacementRequestFile = "replacement-request"
	replacementResultFile  = "replacement-result.env"
)

var (
	ErrReplacementUnavailable    = errors.New("host replacement unit is unavailable")
	ErrInvalidReplacementVersion = errors.New("invalid replacement version")
)

// ReplacementResult is the host executor's last durable outcome.
type ReplacementResult struct {
	Status           string
	RequestedVersion string
	Reason           string
}

// ReplacementStatus is the deployment state that the console may expose.
type ReplacementStatus struct {
	UnitAvailable bool
	LastResult    *ReplacementResult
}

// Replacement owns the application's narrow update boundary: version-only intent files.
type Replacement struct {
	stateDirectory string
}

// NewReplacement creates the state-directory adapter for host replacement requests.
func NewReplacement(stateDirectory string) *Replacement {
	return &Replacement{stateDirectory: stateDirectory}
}

// Status reads host-owned replacement facts without inspecting systemd or a container runtime.
func (r *Replacement) Status() ReplacementStatus {
	result, err := r.Result()
	if err != nil {
		result = nil
	}
	return ReplacementStatus{UnitAvailable: r.UnitAvailable(), LastResult: result}
}

// UnitAvailable reports only the installed host-unit marker, never a guessed deployment mode.
func (r *Replacement) UnitAvailable() bool {
	if r == nil || r.stateDirectory == "" {
		return false
	}
	contents, err := os.ReadFile(filepath.Join(r.stateDirectory, replacementUnitFile))
	return err == nil && string(contents) == "available=yes\n"
}

// Request writes one atomically replaced, version-only intent file for the host executor.
func (r *Replacement) Request(version string) error {
	if !validReplacementVersion(version) {
		return ErrInvalidReplacementVersion
	}
	if !r.UnitAvailable() {
		return ErrReplacementUnavailable
	}
	temporary, err := os.CreateTemp(r.stateDirectory, ".replacement-request-")
	if err != nil {
		return fmt.Errorf("create replacement request: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(version + "\n"); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write replacement request: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync replacement request: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close replacement request: %w", err)
	}
	requestPath := filepath.Join(r.stateDirectory, replacementRequestFile)
	if err := os.Rename(temporaryPath, requestPath); err != nil {
		return fmt.Errorf("replace replacement request: %w", err)
	}
	directory, err := os.Open(r.stateDirectory)
	if err != nil {
		return fmt.Errorf("open replacement request directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync replacement request directory: %w", err)
	}
	return nil
}

// Result reads a syntactically complete result produced by the host executor.
func (r *Replacement) Result() (*ReplacementResult, error) {
	if r == nil || r.stateDirectory == "" {
		return nil, nil
	}
	contents, err := os.ReadFile(filepath.Join(r.stateDirectory, replacementResultFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read replacement result: %w", err)
	}
	return parseReplacementResult(string(contents))
}

func validReplacementVersion(version string) bool {
	if len(version) == 0 || len(version) > 128 {
		return false
	}
	for _, character := range version {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func parseReplacementResult(contents string) (*ReplacementResult, error) {
	values := make(map[string]string, 3)
	seen := make(map[string]bool, 3)
	for _, line := range strings.Split(strings.TrimSuffix(contents, "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || seen[key] {
			return nil, errors.New("replacement result is malformed")
		}
		seen[key] = true
		switch key {
		case "status", "requested_version", "reason":
			values[key] = value
		default:
			return nil, errors.New("replacement result has an unknown field")
		}
	}
	result := &ReplacementResult{
		Status:           values["status"],
		RequestedVersion: values["requested_version"],
		Reason:           values["reason"],
	}
	if !validReplacementStatus(result.Status) ||
		(result.RequestedVersion != "invalid" && !validReplacementVersion(result.RequestedVersion)) ||
		!validReplacementReason(result.Reason) {
		return nil, errors.New("replacement result has an invalid value")
	}
	return result, nil
}

func validReplacementStatus(status string) bool {
	switch status {
	case "applied", "rolled_back", "rollback_failed", "failed", "rejected":
		return true
	default:
		return false
	}
}

func validReplacementReason(reason string) bool {
	return validReplacementVersion(reason)
}
