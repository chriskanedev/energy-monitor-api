package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	httpClient *http.Client
}

type Status struct {
	EMeters []EMeter `json:"emeters"`
}

type EMeter struct {
	Power         float64 `json:"power"`
	Reactive      float64 `json:"reactive"`
	PowerFactor   float64 `json:"pf"`
	Voltage       float64 `json:"voltage"`
	IsValid       bool    `json:"is_valid"`
	Total         float64 `json:"total"`
	TotalReturned float64 `json:"total_returned"`
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{httpClient: httpClient}
}

func (c *Client) FetchStatus(ctx context.Context, host string) (Status, error) {
	endpoint, err := statusURL(host)
	if err != nil {
		return Status{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Status{}, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Status{}, fmt.Errorf("fetch shelly status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Status{}, fmt.Errorf("shelly status returned %s", resp.Status)
	}

	var status Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return Status{}, fmt.Errorf("decode shelly status: %w", err)
	}

	return status, nil
}

func statusURL(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host is empty")
	}

	if !strings.Contains(host, "://") {
		host = "http://" + host
	}

	parsed, err := url.Parse(host)
	if err != nil {
		return "", fmt.Errorf("parse host: %w", err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/status"
	parsed.RawQuery = ""

	return parsed.String(), nil
}
