package lookup

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// kernelReleasesURL is the machine-readable listing kernel.org publishes for its front page.
const kernelReleasesURL = "https://www.kernel.org/releases.json"

// kernelReleasesLimit bounds the response; the real document is a few kilobytes.
const kernelReleasesLimit = 256 * 1024

// kernelReleaseTTL keeps the listing briefly. Releases appear a few times a week, so a short
// cache spares kernel.org a request per query without ever showing a stale week.
const kernelReleaseTTL = 30 * time.Minute

type KernelRelease struct {
	Moniker  string `json:"moniker"`
	Version  string `json:"version"`
	IsEOL    bool   `json:"iseol"`
	Released struct {
		ISODate string `json:"isodate"`
	} `json:"released"`
}

type kernelReleases struct {
	Releases []KernelRelease `json:"releases"`
}

func FetchKernelReleases(ctx context.Context) ([]KernelRelease, bool) {
	if cached, ok := kernelCacheGet(); ok {
		return cached, true
	}
	c, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	body, err := httpGetBody(c, kernelReleasesURL, kernelReleasesLimit)
	if err != nil {
		return nil, false
	}
	var parsed kernelReleases
	if err := json.Unmarshal(body, &parsed); err != nil || len(parsed.Releases) == 0 {
		return nil, false
	}
	kernelCachePut(parsed.Releases)
	return parsed.Releases, true
}

// The cache is one small slice guarded by its own mutex; it never grows.
var (
	kernelCacheMu   sync.Mutex
	kernelCacheAt   time.Time
	kernelCacheData []KernelRelease
)

func kernelCacheGet() ([]KernelRelease, bool) {
	kernelCacheMu.Lock()
	defer kernelCacheMu.Unlock()
	if kernelCacheData == nil || time.Since(kernelCacheAt) > kernelReleaseTTL {
		return nil, false
	}
	return kernelCacheData, true
}

func kernelCachePut(releases []KernelRelease) {
	kernelCacheMu.Lock()
	defer kernelCacheMu.Unlock()
	kernelCacheData = releases
	kernelCacheAt = time.Now()
}
