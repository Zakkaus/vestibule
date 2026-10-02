package lookup

import (
	"context"
	"sync"
)

type PackageHit struct{ Atom, Version, URL string }
type OverlayResult struct {
	Name string
	Hits []PackageHit
}
type PackageResult struct {
	Official     []string
	Versions     map[string][2]string
	Overlays     []OverlayResult
	Availability PkgLookupAvailability
}

func QueryPackages(ctx context.Context, q string) PackageResult {
	overlayOK := PkgC.Refresh(ctx)
	ovRes := PkgC.Search(q)
	mainRes, mainOK := SearchMainTree(ctx, q)
	vm := map[string][2]string{}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, atom := range mainRes {
		wg.Add(1)
		go func(atom string) {
			defer wg.Done()
			stable, latest, _ := PkgVersion(ctx, atom)
			mu.Lock()
			vm[atom] = [2]string{stable, latest}
			mu.Unlock()
		}(atom)
	}
	wg.Wait()
	result := PackageResult{Official: mainRes, Versions: vm, Availability: PkgLookupAvailability{Official: mainOK, Overlays: overlayOK}}
	for _, o := range Overlays {
		hits := ovRes[o.Name]
		if len(hits) == 0 {
			continue
		}
		source := OverlayResult{Name: o.Name, Hits: make([]PackageHit, 0, len(hits))}
		for _, atom := range hits {
			source.Hits = append(source.Hits, PackageHit{Atom: atom, Version: PkgC.OverlayVer(o.Name, atom), URL: o.TreeURL(atom)})
		}
		result.Overlays = append(result.Overlays, source)
	}
	return result
}

type OverlayReference struct{ Name, URL string }
type UseResult struct {
	Atoms        []string
	Atom         string
	Info         PkgFullInfo
	Found        bool
	Source, URL  string
	Overlay      bool
	AlsoIn       []OverlayReference
	Availability PkgLookupAvailability
}

func QueryUse(ctx context.Context, q string) UseResult {
	overlayOK := PkgC.Refresh(ctx)
	srcs, availability := ResolveUseSources(ctx, q, overlayOK)
	result := UseResult{Availability: availability, Atoms: make([]string, 0, len(srcs))}
	for atom := range srcs {
		result.Atoms = append(result.Atoms, atom)
	}
	if len(srcs) != 1 {
		return result
	}
	atom := result.Atoms[0]
	source := srcs[atom]
	result.Atom = atom
	alsoIn := source.Ovs
	if source.Official {
		if info, found, _ := OfficialInfo(ctx, atom); found {
			result.Info, result.Found = info, true
			result.URL = "https://packages.gentoo.org/packages/" + atom
		}
	}
	if !result.Found && len(source.Ovs) > 0 {
		name := source.Ovs[0]
		overlay, _ := OverlayByName(name)
		if info, ok := OverlayInfo(ctx, overlay, atom, PkgC.OverlayVer(name, atom)); ok {
			result.Info, result.Found = info, true
			result.Source, result.URL, result.Overlay = "overlay:"+name, overlay.TreeURL(atom), true
			alsoIn = source.Ovs[1:]
		}
	}
	for _, name := range alsoIn {
		ref := OverlayReference{Name: name}
		if overlay, ok := OverlayByName(name); ok {
			ref.URL = overlay.TreeURL(atom)
		}
		result.AlsoIn = append(result.AlsoIn, ref)
	}
	return result
}
