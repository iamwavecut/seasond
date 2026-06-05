package policy

import (
	"testing"
	"time"
)

func TestParsePrefixUsesAgeOverride(t *testing.T) {
	got, remaining, err := ParsePrefix("/age/14d/github.com/pkg/errors/@v/list", 336*time.Hour)
	if err != nil {
		t.Fatalf("ParsePrefix returned error: %v", err)
	}
	if got.MinAge != 14*24*time.Hour {
		t.Fatalf("MinAge = %s, want 336h", got.MinAge)
	}
	if remaining != "/github.com/pkg/errors/@v/list" {
		t.Fatalf("remaining = %q, want module path", remaining)
	}
}

func TestParsePrefixFallsBackToDefaultAtRoot(t *testing.T) {
	got, remaining, err := ParsePrefix("/github.com/pkg/errors/@v/list", 48*time.Hour)
	if err != nil {
		t.Fatalf("ParsePrefix returned error: %v", err)
	}
	if got.MinAge != 48*time.Hour {
		t.Fatalf("MinAge = %s, want default", got.MinAge)
	}
	if remaining != "/github.com/pkg/errors/@v/list" {
		t.Fatalf("remaining = %q, want original path", remaining)
	}
}

func TestAllowedBlocksUnknownAndFreshVersions(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	p := Policy{MinAge: 14 * 24 * time.Hour}

	if decision := p.Decide(nil, now); decision.Allowed {
		t.Fatalf("unknown version allowed, want blocked")
	}

	fresh := now.Add(-13 * 24 * time.Hour)
	if decision := p.Decide(&fresh, now); decision.Allowed {
		t.Fatalf("fresh version allowed, want blocked")
	}

	old := now.Add(-15 * 24 * time.Hour)
	if decision := p.Decide(&old, now); !decision.Allowed {
		t.Fatalf("old version blocked, want allowed: %s", decision.Reason)
	}
}
