package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

type indexRow struct {
	Path      string    `json:"Path"`
	Version   string    `json:"Version"`
	Timestamp time.Time `json:"Timestamp"`
}

type pair struct {
	module       string
	allowed      indexRow
	blocked      indexRow
	bootstrap    time.Time
	minAge       time.Duration
	selectedPage int
}

func main() {
	lookbackDays := flag.Int("lookback-days", 45, "days of Go index history to scan")
	maxPages := flag.Int("max-pages", 80, "maximum index pages to scan")
	prefixesRaw := flag.String("prefixes", "cloud.google.com/go/,google.golang.org/genproto", "comma-separated module prefixes")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	prefixes := splitPrefixes(*prefixesRaw)
	client := &http.Client{Timeout: 30 * time.Second}
	now := time.Now().UTC()
	since := now.Add(-time.Duration(*lookbackDays) * 24 * time.Hour)

	got, err := findPair(ctx, client, now, since, *maxPages, prefixes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "select Go index pair: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("MODULE_PATH=%s\n", shellQuote(got.module))
	fmt.Printf("ALLOWED_VERSION=%s\n", shellQuote(got.allowed.Version))
	fmt.Printf("ALLOWED_TIMESTAMP=%s\n", shellQuote(got.allowed.Timestamp.UTC().Format(time.RFC3339Nano)))
	fmt.Printf("BLOCKED_VERSION=%s\n", shellQuote(got.blocked.Version))
	fmt.Printf("BLOCKED_TIMESTAMP=%s\n", shellQuote(got.blocked.Timestamp.UTC().Format(time.RFC3339Nano)))
	fmt.Printf("BOOTSTRAP_SINCE=%s\n", shellQuote(got.bootstrap.UTC().Format(time.RFC3339Nano)))
	fmt.Printf("MIN_AGE=%s\n", shellQuote(durationSeconds(got.minAge)))
	fmt.Printf("INDEX_PAGE=%s\n", shellQuote(fmt.Sprint(got.selectedPage)))
}

func findPair(ctx context.Context, client *http.Client, now, since time.Time, maxPages int, prefixes []string) (pair, error) {
	for page := 1; page <= maxPages; page++ {
		rows, err := fetchPage(ctx, client, since)
		if err != nil {
			return pair{}, err
		}
		if len(rows) == 0 {
			break
		}
		if got, ok := choosePair(ctx, client, now, rows, page, prefixes); ok {
			return got, nil
		}

		next := rows[len(rows)-1].Timestamp.UTC().Add(time.Nanosecond)
		if !next.After(since) {
			break
		}
		since = next
	}
	return pair{}, fmt.Errorf("no usable Google module pair found in %d index pages", maxPages)
}

func fetchPage(ctx context.Context, client *http.Client, since time.Time) ([]indexRow, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://index.golang.org/index?since="+since.UTC().Format(time.RFC3339Nano), nil)
	if err != nil {
		return nil, fmt.Errorf("create index request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch index: status %d", resp.StatusCode)
	}

	var rows []indexRow
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var row indexRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("decode index row: %w", err)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read index response: %w", err)
	}
	return rows, nil
}

func choosePair(ctx context.Context, client *http.Client, now time.Time, rows []indexRow, page int, prefixes []string) (pair, bool) {
	byPath := make(map[string][]indexRow)
	for _, row := range rows {
		if eligiblePath(row.Path, prefixes) && semver.IsValid(row.Version) {
			byPath[row.Path] = append(byPath[row.Path], row)
		}
	}

	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	slices.Sort(paths)

	for _, path := range paths {
		candidates := byPath[path]
		slices.SortFunc(candidates, func(a, b indexRow) int {
			return a.Timestamp.Compare(b.Timestamp)
		})
		for i := 0; i+1 < len(candidates); i++ {
			allowed := candidates[i]
			blocked := candidates[i+1]
			gap := blocked.Timestamp.Sub(allowed.Timestamp)
			if gap < 10*time.Second {
				continue
			}
			midpoint := allowed.Timestamp.Add(gap / 2)
			minAge := now.Sub(midpoint)
			if minAge <= 0 {
				continue
			}
			if !proxyHasVersion(ctx, client, allowed.Path, allowed.Version) ||
				!proxyHasVersion(ctx, client, blocked.Path, blocked.Version) {
				continue
			}
			return pair{
				module:       path,
				allowed:      allowed,
				blocked:      blocked,
				bootstrap:    allowed.Timestamp.UTC().Add(-time.Second),
				minAge:       minAge,
				selectedPage: page,
			}, true
		}
	}
	return pair{}, false
}

func proxyHasVersion(ctx context.Context, client *http.Client, modulePath, version string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://proxy.golang.org/"+modulePath+"/@v/"+version+".info", nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func splitPrefixes(raw string) []string {
	var prefixes []string
	for _, prefix := range strings.Split(raw, ",") {
		prefix = strings.TrimSpace(prefix)
		if prefix != "" {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
}

func eligiblePath(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func durationSeconds(d time.Duration) string {
	seconds := int64(d.Round(time.Second) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return fmt.Sprintf("%ds", seconds)
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
