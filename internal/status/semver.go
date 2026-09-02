package status

import (
	"strconv"
	"strings"
)

type semanticVersion struct {
	major      uint64
	minor      uint64
	patch      uint64
	prerelease []string
}

func newerRelease(current, latest string) bool {
	latestVersion, latestOK := parseSemanticVersion(latest)
	if !latestOK {
		return false
	}
	currentVersion, currentOK := parseSemanticVersion(current)
	if !currentOK {
		return current != latest
	}
	return compareSemanticVersions(currentVersion, latestVersion) < 0
}

func parseSemanticVersion(raw string) (semanticVersion, bool) {
	value := strings.TrimPrefix(raw, "v")
	if value == "" {
		return semanticVersion{}, false
	}
	withoutBuild, build, hasBuild := strings.Cut(value, "+")
	if hasBuild && !validIdentifiers(build, false) {
		return semanticVersion{}, false
	}
	core, prerelease, hasPrerelease := strings.Cut(withoutBuild, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	major, majorOK := parseCoreVersionPart(parts[0])
	minor, minorOK := parseCoreVersionPart(parts[1])
	patch, patchOK := parseCoreVersionPart(parts[2])
	if !majorOK || !minorOK || !patchOK {
		return semanticVersion{}, false
	}
	parsed := semanticVersion{major: major, minor: minor, patch: patch}
	if hasPrerelease {
		if !validIdentifiers(prerelease, true) {
			return semanticVersion{}, false
		}
		parsed.prerelease = strings.Split(prerelease, ".")
	}
	return parsed, true
}

func parseCoreVersionPart(raw string) (uint64, bool) {
	if raw == "" || len(raw) > 1 && raw[0] == '0' {
		return 0, false
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	return value, err == nil
}

func validIdentifiers(raw string, rejectNumericLeadingZero bool) bool {
	for _, identifier := range strings.Split(raw, ".") {
		if identifier == "" || rejectNumericLeadingZero && numericLeadingZero(identifier) {
			return false
		}
		for _, character := range identifier {
			if character != '-' && (character < '0' || character > '9') &&
				(character < 'A' || character > 'Z') && (character < 'a' || character > 'z') {
				return false
			}
		}
	}
	return true
}

func numericLeadingZero(identifier string) bool {
	if len(identifier) < 2 || identifier[0] != '0' {
		return false
	}
	for _, character := range identifier {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func compareSemanticVersions(left, right semanticVersion) int {
	if compared := compareUint64(left.major, right.major); compared != 0 {
		return compared
	}
	if compared := compareUint64(left.minor, right.minor); compared != 0 {
		return compared
	}
	if compared := compareUint64(left.patch, right.patch); compared != 0 {
		return compared
	}
	return comparePrerelease(left.prerelease, right.prerelease)
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	for index := 0; index < len(left) && index < len(right); index++ {
		if compared := comparePrereleaseIdentifier(left[index], right[index]); compared != 0 {
			return compared
		}
	}
	return compareUint64(uint64(len(left)), uint64(len(right)))
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumeric := identifierIsNumeric(left)
	rightNumeric := identifierIsNumeric(right)
	if leftNumeric && rightNumeric {
		if len(left) < len(right) {
			return -1
		}
		if len(left) > len(right) {
			return 1
		}
	}
	if leftNumeric != rightNumeric {
		if leftNumeric {
			return -1
		}
		return 1
	}
	return strings.Compare(left, right)
}

func identifierIsNumeric(identifier string) bool {
	for _, character := range identifier {
		if character < '0' || character > '9' {
			return false
		}
	}
	return identifier != ""
}

func compareUint64(left, right uint64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
