package metrics

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

type Registry struct {
	mu        sync.Mutex
	requests  map[string]uint64
	blocked   map[string]uint64
	durations map[string]time.Duration
}

func New() *Registry {
	return &Registry{
		requests:  make(map[string]uint64),
		blocked:   make(map[string]uint64),
		durations: make(map[string]time.Duration),
	}
}

func (r *Registry) ObserveRequest(endpoint string, status int, decision string, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := labels(endpoint, status, decision)
	r.requests[key]++
	r.durations[key] += duration
	if decision == "block" {
		r.blocked["policy"]++
	}
}

func (r *Registry) WritePrometheus(w io.Writer, knownVersions int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	_, _ = fmt.Fprintln(w, "# TYPE seasond_requests_total counter")
	for _, key := range sortedKeys(r.requests) {
		_, _ = fmt.Fprintf(w, "seasond_requests_total{%s} %d\n", key, r.requests[key])
	}
	_, _ = fmt.Fprintln(w, "# TYPE seasond_request_duration_seconds summary")
	for _, key := range sortedKeys(r.durations) {
		_, _ = fmt.Fprintf(w, "seasond_request_duration_seconds_sum{%s} %.6f\n", key, r.durations[key].Seconds())
	}
	_, _ = fmt.Fprintln(w, "# TYPE seasond_blocked_requests_total counter")
	for _, reason := range sortedKeys(r.blocked) {
		_, _ = fmt.Fprintf(w, "seasond_blocked_requests_total{reason=%q} %d\n", reason, r.blocked[reason])
	}
	_, _ = fmt.Fprintln(w, "# TYPE seasond_known_versions_total gauge")
	_, _ = fmt.Fprintf(w, "seasond_known_versions_total %d\n", knownVersions)
}

func labels(endpoint string, status int, decision string) string {
	return fmt.Sprintf("endpoint=%q,status=%q,decision=%q", endpoint, fmt.Sprint(status), decision)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
