package lookup

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

type ArmState uint8

const (
	ArmQueryFailed ArmState = iota
	ArmNotInOfficialTree
	ArmKeywords
	ArmNoPackage
	ArmAvailable
	ArmNotInFedora
	ArmFedoraFailed
	ArmFedoraAvailable
	ArmPKGBUILDParseFailed
	ArmAny
	ArmAarch64
	Arm32Only
	ArmX86Only
	ArmNotInAUR
	ArmAURFailed
	ArmNotPackaged
	ArmPackaged
)

type ArmSupport struct {
	State                                ArmState
	Stable, Testing, URL, Suite, Version string
	Development                          bool
}

func GentooArmStatus(ctx context.Context, name string) ArmSupport {
	return gentooArmStatusWith(ctx, name, SearchMainTree, ArmStatus)
}

func gentooArmStatusWith(ctx context.Context, name string, search func(context.Context, string) ([]string, bool), status func(context.Context, string) (string, string, bool)) ArmSupport {
	result := ArmSupport{URL: "https://packages.gentoo.org/packages/search?q=" + url.QueryEscape(name)}
	atoms, available := search(ctx, name)
	if !available {
		return result
	}
	if len(atoms) == 0 {
		result.State = ArmNotInOfficialTree
		return result
	}
	result.URL = "https://packages.gentoo.org/packages/" + atoms[0]
	stable, testing, ok := status(ctx, atoms[0])
	if !ok {
		return result
	}
	result.State, result.Stable, result.Testing = ArmKeywords, stable, testing
	return result
}

func MadisonArmStatus(ctx context.Context, madisonURL, pkg string, devSuite func(string) bool) ArmSupport {
	body, err := httpGetBody(ctx, madisonURL+url.QueryEscape(pkg)+"&text=on&a=arm64", 1<<20)
	if err != nil {
		return ArmSupport{State: ArmQueryFailed}
	}
	entries := parseMadison(string(body))
	if len(entries) == 0 {
		return ArmSupport{State: ArmNoPackage}
	}
	suite, ver, dev := pickMadison(entries, devSuite)
	return ArmSupport{State: ArmAvailable, Suite: suite, Version: DisplayVer(ver), Development: dev}
}

func FedoraArmStatus(ctx context.Context, pkg string) ArmSupport {
	return fedoraArmStatusWith(ctx, pkg, func(ctx context.Context, endpoint string) (string, error) {
		var r struct {
			Version string `json:"version"`
			Arch    string `json:"arch"`
		}
		if err := GetJSON(ctx, endpoint, nil, &r); err != nil {
			return "", err
		}
		if r.Arch != "aarch64" && r.Arch != "noarch" {
			return "", fmt.Errorf("fedora mdapi returned architecture %q", r.Arch)
		}
		return r.Version, nil
	})
}

func fedoraArmStatusWith(ctx context.Context, pkg string, fetch func(context.Context, string) (string, error)) ArmSupport {
	version, err := fetch(ctx, "https://mdapi.fedoraproject.org/rawhide/pkg/"+url.PathEscape(pkg))
	if err != nil {
		if httpStatusCode(err) == 404 {
			return ArmSupport{State: ArmNotInFedora}
		}
		return ArmSupport{State: ArmFedoraFailed}
	}
	if version == "" {
		return ArmSupport{State: ArmFedoraFailed}
	}
	return ArmSupport{State: ArmFedoraAvailable, Version: version}
}

func aurArchLabel(pkgbuild string) ArmSupport {
	match := aurArchRe.FindStringSubmatch(pkgbuild)
	if match == nil {
		return ArmSupport{State: ArmPKGBUILDParseFailed}
	}
	arch := strings.ToLower(match[1])
	switch {
	case strings.Contains(arch, "any"):
		return ArmSupport{State: ArmAny}
	case strings.Contains(arch, "aarch64"):
		return ArmSupport{State: ArmAarch64}
	case strings.Contains(arch, "arm"):
		return ArmSupport{State: Arm32Only}
	default:
		return ArmSupport{State: ArmX86Only}
	}
}

func AurArmStatus(ctx context.Context, pkg string) ArmSupport {
	body, err := httpGetBody(ctx, "https://aur.archlinux.org/cgit/aur.git/plain/PKGBUILD?h="+url.QueryEscape(pkg), 64<<10)
	if err != nil {
		if httpStatusCode(err) == 404 {
			return ArmSupport{State: ArmNotInAUR}
		}
		return ArmSupport{State: ArmAURFailed}
	}
	return aurArchLabel(string(body))
}

func AlarmArmStatus(ctx context.Context, pkg string) ArmSupport {
	resp, err := httpGet(ctx, "https://archlinuxarm.org/packages/aarch64/"+url.PathEscape(pkg), nil)
	if err == nil {
		err = resp.Body.Close()
	}
	if err != nil {
		if httpStatusCode(err) == 404 {
			return ArmSupport{State: ArmNotPackaged}
		}
		return ArmSupport{State: ArmQueryFailed}
	}
	return ArmSupport{State: ArmPackaged}
}

type ArmLookup struct {
	Atom, URL, Stable, Testing          string
	Available, Found, KeywordsAvailable bool
}

func LookupArm(ctx context.Context, name string, search func(context.Context, string) ([]string, bool), status func(context.Context, string) (string, string, bool)) ArmLookup {
	atoms, available := search(ctx, name)
	result := ArmLookup{Available: available}
	if !available || len(atoms) == 0 {
		return result
	}
	result.Found, result.Atom = true, atoms[0]
	result.URL = "https://packages.gentoo.org/packages/" + result.Atom
	result.Stable, result.Testing, result.KeywordsAvailable = status(ctx, result.Atom)
	return result
}

type ArmRow struct {
	Label, URL string
	Support    ArmSupport
}

func QueryArmPackages(ctx context.Context, name string) []ArmRow {
	EnsureReleaseInfo(ctx, time.Now())
	pe := url.PathEscape(name)
	sources := []struct {
		label, url string
		query      func() ArmSupport
	}{
		{"Gentoo", "", func() ArmSupport { return GentooArmStatus(ctx, name) }},
		{"Debian", "https://tracker.debian.org/pkg/" + pe, func() ArmSupport {
			return MadisonArmStatus(ctx, "https://qa.debian.org/madison.php?package=", name, DebianDevSuite)
		}},
		{"Ubuntu", "https://launchpad.net/ubuntu/+source/" + pe, func() ArmSupport {
			return MadisonArmStatus(ctx, "https://people.canonical.com/~ubuntu-archive/madison.cgi?package=", name, UbuntuDevSuite)
		}},
		{"Fedora", "https://packages.fedoraproject.org/pkgs/" + pe + "/", func() ArmSupport { return FedoraArmStatus(ctx, name) }},
		{"Arch Linux ARM", "https://archlinuxarm.org/packages/aarch64/" + pe, func() ArmSupport { return AlarmArmStatus(ctx, name) }},
		{"AUR", "https://aur.archlinux.org/packages/" + pe, func() ArmSupport { return AurArmStatus(ctx, name) }},
	}
	results := make([]ArmRow, len(sources))
	var wg sync.WaitGroup
	for i, source := range sources {
		wg.Add(1)
		go func(i int, source struct {
			label, url string
			query      func() ArmSupport
		}) {
			defer wg.Done()
			support := source.query()
			endpoint := source.url
			if endpoint == "" {
				endpoint = support.URL
			}
			results[i] = ArmRow{Label: source.label, URL: endpoint, Support: support}
		}(i, source)
	}
	wg.Wait()
	return results
}
