package upstream

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	base       string
	httpClient *http.Client
}

func New(base string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		base:       strings.TrimRight(base, "/"),
		httpClient: httpClient,
	}
}

func (c *Client) List(ctx context.Context, modulePath string) ([]string, int, error) {
	body, status, _, err := c.get(ctx, modulePath+"/@v/list")
	if err != nil || status != http.StatusOK {
		return nil, status, err
	}
	lines := strings.Split(string(body), "\n")
	versions := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			versions = append(versions, line)
		}
	}
	return versions, status, nil
}

func (c *Client) Info(ctx context.Context, modulePath, version string) ([]byte, string, int, error) {
	body, status, contentType, err := c.get(ctx, modulePath+"/@v/"+version+".info")
	return body, contentType, status, err
}

func (c *Client) ArtifactURL(modulePath, version, extension string) string {
	return c.url(modulePath + "/@v/" + version + extension)
}

func (c *Client) get(ctx context.Context, path string) ([]byte, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return nil, 0, "", fmt.Errorf("create upstream request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, "", fmt.Errorf("fetch upstream: %w", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, resp.StatusCode, resp.Header.Get("Content-Type"), fmt.Errorf("read upstream body: %w", readErr)
	}
	return body, resp.StatusCode, resp.Header.Get("Content-Type"), nil
}

func (c *Client) url(path string) string {
	return c.base + "/" + strings.TrimLeft(path, "/")
}
