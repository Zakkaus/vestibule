package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/api"
	"github.com/Zakkaus/vestibule/internal/store"
)

const claimStateFile = "claim.json"

type claimRecord struct {
	SetupTokenHash string `json:"setup_token_hash,omitempty"`
	BotToken       string `json:"bot_token,omitempty"`
}

type setupState struct {
	mu     sync.Mutex
	path   string
	record claimRecord
}

type setupCoordinator struct {
	mu        sync.Mutex
	root      context.Context
	options   Options
	runtime   *services
	state     *setupState
	progress  chan<- struct{}
	activated chan<- *activeRuntime
	stopped   bool
}

type setupActivationError struct {
	cause error
}

func (e setupActivationError) Error() string { return "instance activation failed" }
func (e setupActivationError) Unwrap() error { return e.cause }
func openSetupState(stateDirectory, setupToken string) (*setupState, error) {
	state := &setupState{}
	if stateDirectory == "" {
		return state, nil
	}
	state.path = filepath.Join(stateDirectory, claimStateFile)
	if err := verifyClaimFileMode(state.path); err != nil {
		return nil, err
	}
	if err := store.Load(state.path, &state.record); err != nil {
		return nil, fmt.Errorf("load claim state: %w", err)
	}
	setupToken = strings.TrimSpace(setupToken)
	if state.record.BotToken != "" || setupToken == "" {
		return state, nil
	}
	hash := hashSetupToken(setupToken)
	if state.record.SetupTokenHash != "" && !constantTimeEqual(state.record.SetupTokenHash, hash) {
		return nil, errors.New("configured setup token does not match claim state")
	}
	if state.record.SetupTokenHash == "" {
		state.record.SetupTokenHash = hash
		if err := state.saveLocked(); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func verifyClaimFileMode(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat claim state: %w", err)
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("claim state permissions are %04o, want 0600", info.Mode().Perm())
	}
	return nil
}

func (s *setupState) BotToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.record.BotToken)
}

func (s *setupState) setupAvailable(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path != "" && s.record.BotToken == "" && s.record.SetupTokenHash != "" &&
		constantTimeEqual(s.record.SetupTokenHash, hashSetupToken(token))
}

func (s *setupState) stage(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" || s.record.BotToken != "" || s.record.SetupTokenHash == "" {
		return api.ErrSetupUnavailable
	}
	previous := s.record
	s.record.BotToken = token
	if err := s.saveLocked(); err != nil {
		s.record = previous
		return err
	}
	return nil
}

func (s *setupState) rollback() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.record
	s.record.BotToken = ""
	if err := s.saveLocked(); err != nil {
		s.record = previous
		return err
	}
	return nil
}

func (s *setupState) complete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.record
	s.record.SetupTokenHash = ""
	if err := s.saveLocked(); err != nil {
		s.record = previous
		return err
	}
	return nil
}

func (s *setupState) saveLocked() error {
	if err := store.Write(s.path, s.record); err != nil {
		return fmt.Errorf("save claim state: %w", err)
	}
	return nil
}

func hashSetupToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}

func constantTimeEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func newSetupCoordinator(
	root context.Context,
	options Options,
	runtime *services,
	state *setupState,
	progress chan<- struct{},
	activated chan<- *activeRuntime,
) *setupCoordinator {
	return &setupCoordinator{
		root: root, options: options, runtime: runtime, state: state, progress: progress, activated: activated,
	}
}

func (c *setupCoordinator) SetupAvailable(token string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.stopped && c.state.BotToken() == "" && c.state.setupAvailable(token)
}

func (c *setupCoordinator) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
}
func (c *setupCoordinator) Claim(_ context.Context, token string) (api.SetupResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.state.BotToken() != "" {
		return api.SetupResult{}, api.ErrSetupUnavailable
	}
	if strings.TrimSpace(token) == "" {
		return api.SetupResult{}, setupActivationError{cause: errors.New("empty bot token")}
	}
	if err := c.state.stage(token); err != nil {
		return api.SetupResult{}, err
	}
	options := c.options
	options.Token = token
	if err := activateServices(c.root, c.runtime, options, c.progress); err != nil {
		log.Printf("setup claim failed during activation: %v", err)
		return api.SetupResult{}, c.rollback(err)
	}
	bindingURL, err := ownerBindingURL(c.runtime)
	if err != nil {
		return api.SetupResult{}, c.rollback(err)
	}
	active, err := startActiveRuntime(c.root, c.runtime)
	if err != nil {
		return api.SetupResult{}, c.rollback(err)
	}
	if err := c.state.complete(); err != nil {
		stopActiveRuntime(active)
		return api.SetupResult{}, c.rollback(err)
	}
	c.activated <- active
	return api.SetupResult{Claimed: true, BotUsername: c.runtime.identity.Username, BindingURL: bindingURL}, nil
}

func (c *setupCoordinator) rollback(cause error) error {
	if err := c.state.rollback(); err != nil {
		return setupActivationError{cause: err}
	}
	return setupActivationError{cause: cause}
}

func ownerBindingURL(runtime *services) (string, error) {
	nonce, _, err := runtime.settings.EnsureOwnerClaim(time.Now(), runtime.cfg.OwnerClaimLifetime())
	if err != nil {
		return "", fmt.Errorf("create owner binding: %w", err)
	}
	if nonce == "" {
		return "", nil
	}
	return fmt.Sprintf("https://t.me/%s?start=owner_%s", runtime.identity.Username, nonce), nil
}
