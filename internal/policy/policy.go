package policy

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Policy struct {
	MinAge time.Duration
}

type Decision struct {
	Allowed bool
	Reason  string
}

func ParsePrefix(requestPath string, defaultMinAge time.Duration) (Policy, string, error) {
	if !strings.HasPrefix(requestPath, "/age/") {
		return Policy{MinAge: defaultMinAge}, requestPath, nil
	}

	rest := strings.TrimPrefix(requestPath, "/age/")
	rawAge, remaining, ok := strings.Cut(rest, "/")
	if !ok || rawAge == "" {
		return Policy{}, "", fmt.Errorf("invalid age policy prefix")
	}

	minAge, err := parseAge(rawAge)
	if err != nil {
		return Policy{}, "", fmt.Errorf("invalid age policy %q: %w", rawAge, err)
	}
	if minAge <= 0 {
		return Policy{}, "", fmt.Errorf("age policy must be positive")
	}

	return Policy{MinAge: minAge}, "/" + remaining, nil
}

func (p Policy) Decide(firstSeenAt *time.Time, now time.Time) Decision {
	if firstSeenAt == nil {
		return Decision{Allowed: false, Reason: "unknown_first_seen"}
	}
	if now.Sub(*firstSeenAt) < p.MinAge {
		return Decision{Allowed: false, Reason: "too_fresh"}
	}
	return Decision{Allowed: true, Reason: "age_gate_passed"}
}

func parseAge(raw string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(raw, "d"); ok {
		n, err := strconv.ParseInt(days, 10, 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(raw)
}
