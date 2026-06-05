package semverutil

import "testing"

func TestLatestModuleVersionIgnoresInvalidVersions(t *testing.T) {
	got, ok := Latest([]string{"v1.0.0", "not-a-version", "v1.3.0", "v1.3.0-pre", "v2.0.0"})
	if !ok {
		t.Fatal("Latest returned ok=false")
	}
	if got != "v2.0.0" {
		t.Fatalf("Latest = %q, want v2.0.0", got)
	}
}

func TestSortModuleVersionsUsesGoSemverOrdering(t *testing.T) {
	got := Sort([]string{"v1.2.0", "v1.10.0", "v1.2.0-pre", "bad", "v1.0.0"})
	want := []string{"v1.0.0", "v1.2.0-pre", "v1.2.0", "v1.10.0"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q; all=%#v", i, got[i], want[i], got)
		}
	}
}
