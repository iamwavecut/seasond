//go:build docker_e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDockerizedGoToolUsesSeasonDIndexAgeGate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Docker E2E in short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker is not available: %v", err)
	}

	root := repoRoot(t)
	goImage := envDefault("SEASOND_E2E_GO_IMAGE", "golang:1.26-alpine")
	lookbackDays := envDefault("SEASOND_E2E_LOOKBACK_DAYS", "45")
	script := filepath.Join(root, "internal/e2e/testdata/docker-functional.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("Docker functional script is missing: %v", err)
	}

	cmd := exec.CommandContext(t.Context(),
		"docker", "run", "--rm",
		"-v", root+":/work",
		"-w", "/work",
		goImage,
		"sh", "internal/e2e/testdata/docker-functional.sh",
	)
	cmd.Env = append(os.Environ(), "SEASOND_E2E_LOOKBACK_DAYS="+lookbackDays)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dockerized functional test failed: %v\n%s", err, output)
	}
	t.Logf("dockerized functional test output:\n%s", output)
}

func repoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	root, err := filepath.Abs(filepath.Join(wd, "../.."))
	if err != nil {
		t.Fatalf("Abs returned error: %v", err)
	}
	return root
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
