package semverutil

import (
	"slices"
	"sort"

	"golang.org/x/mod/semver"
)

func Latest(versions []string) (string, bool) {
	sorted := Sort(versions)
	if len(sorted) == 0 {
		return "", false
	}
	return sorted[len(sorted)-1], true
}

func Sort(versions []string) []string {
	valid := make([]string, 0, len(versions))
	for _, version := range versions {
		if semver.IsValid(version) {
			valid = append(valid, version)
		}
	}
	sort.Slice(valid, func(i, j int) bool {
		return semver.Compare(valid[i], valid[j]) < 0
	})
	return slices.Clip(valid)
}
