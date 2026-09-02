package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Zakkaus/vestibule/migrations"
)

const (
	latestReleaseURL      = "https://api.github.com/repos/Zakkaus/vestibule/releases/latest"
	releaseManifestName   = "vestibule-schema-manifest"
	releaseResponseLimit  = 1 << 20
	releaseManifestLimit  = 4 << 10
	releaseRequestTimeout = 10 * time.Second
)

// ErrReleaseLookupUnavailable marks a release source that could not be read safely.
var ErrReleaseLookupUnavailable = errors.New("release lookup unavailable")

// ReleaseRollback is the structured rollback result for an available release.
type ReleaseRollback struct {
	Available                    bool
	Reason                       migrations.RollbackReason
	TargetSchemaVersion          int
	RetainedSchemaVersion        int
	MinimumRollbackSchemaVersion int
}

// ReleaseInfo is the latest published release and its pre-download rollback assessment.
type ReleaseInfo struct {
	Version         string
	URL             string
	Notes           string
	PublishedAt     time.Time
	UpdateAvailable bool
	Rollback        *ReleaseRollback
}

// ReleaseChecker queries the fixed public release source only when an operator asks.
type ReleaseChecker struct {
	client          *http.Client
	currentVersion  string
	githubToken     string
	currentManifest migrations.SchemaManifest
	manifestErr     error
}

// NewReleaseChecker constructs an on-demand checker without performing network I/O.
func NewReleaseChecker(currentVersion, githubToken string) *ReleaseChecker {
	manifest, err := migrations.CurrentSchemaManifest()
	return &ReleaseChecker{
		client:          &http.Client{Timeout: releaseRequestTimeout},
		currentVersion:  currentVersion,
		githubToken:     githubToken,
		currentManifest: manifest,
		manifestErr:     err,
	}
}

// Latest fetches the latest published release and its schema manifest from the fixed repository.
func (checker *ReleaseChecker) Latest(ctx context.Context) (ReleaseInfo, error) {
	if checker == nil || checker.client == nil || checker.manifestErr != nil {
		return ReleaseInfo{}, fmt.Errorf("%w: current schema manifest is unavailable", ErrReleaseLookupUnavailable)
	}
	release, err := checker.fetchLatestRelease(ctx)
	if err != nil {
		return ReleaseInfo{}, err
	}
	manifest, err := checker.fetchReleaseManifest(ctx, release.TagName)
	if err != nil {
		return ReleaseInfo{}, err
	}
	updateAvailable := newerRelease(checker.currentVersion, release.TagName)
	info := ReleaseInfo{
		Version:         release.TagName,
		URL:             "https://github.com/Zakkaus/vestibule/releases/tag/" + url.PathEscape(release.TagName),
		Notes:           release.Body,
		PublishedAt:     release.PublishedAt.UTC(),
		UpdateAvailable: updateAvailable,
	}
	if updateAvailable {
		assessment := manifest.AssessRollback(checker.currentManifest.TargetSchemaVersion)
		info.Rollback = &ReleaseRollback{
			Available:                    assessment.CanRollback(),
			Reason:                       assessment.Reason,
			TargetSchemaVersion:          assessment.TargetVersion,
			RetainedSchemaVersion:        assessment.RollbackVersion,
			MinimumRollbackSchemaVersion: assessment.MinimumCompatibleVersion,
		}
	}
	return info, nil
}

type githubRelease struct {
	TagName     string    `json:"tag_name"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

func (checker *ReleaseChecker) fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	body, err := checker.get(ctx, latestReleaseURL, releaseResponseLimit, true)
	if err != nil {
		return githubRelease{}, err
	}
	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return githubRelease{}, fmt.Errorf("%w: decode latest release: %v", ErrReleaseLookupUnavailable, err)
	}
	if _, ok := parseSemanticVersion(release.TagName); !ok || release.PublishedAt.IsZero() {
		return githubRelease{}, fmt.Errorf("%w: latest release metadata is incomplete", ErrReleaseLookupUnavailable)
	}
	for _, asset := range release.Assets {
		if asset.Name == releaseManifestName {
			return release, nil
		}
	}
	return githubRelease{}, fmt.Errorf("%w: latest release has no schema manifest", ErrReleaseLookupUnavailable)
}

func (checker *ReleaseChecker) fetchReleaseManifest(
	ctx context.Context,
	version string,
) (migrations.SchemaManifest, error) {
	assetURL := fmt.Sprintf(
		"https://github.com/Zakkaus/vestibule/releases/download/%s/%s",
		url.PathEscape(version),
		releaseManifestName,
	)
	body, err := checker.get(ctx, assetURL, releaseManifestLimit, false)
	if err != nil {
		return migrations.SchemaManifest{}, err
	}
	manifest, err := migrations.ParseSchemaManifest(body)
	if err != nil {
		return migrations.SchemaManifest{}, fmt.Errorf("%w: %v", ErrReleaseLookupUnavailable, err)
	}
	return manifest, nil
}

func (checker *ReleaseChecker) get(
	ctx context.Context,
	address string,
	limit int64,
	authenticated bool,
) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build release request", ErrReleaseLookupUnavailable)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "vestibule-release-check")
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	if authenticated && checker.githubToken != "" {
		request.Header.Set("Authorization", "Bearer "+checker.githubToken)
	}
	response, err := checker.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: request release source: %v", ErrReleaseLookupUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: release source returned HTTP %d", ErrReleaseLookupUnavailable, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read release source: %v", ErrReleaseLookupUnavailable, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: release source response exceeds %d bytes", ErrReleaseLookupUnavailable, limit)
	}
	return body, nil
}
