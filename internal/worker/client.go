package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client is an HTTP client for the FORGE API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type RegisterRequest struct {
	Name         string   `json:"name"`
	Hostname     string   `json:"hostname"`
	PID          int      `json:"pid"`
	Capabilities []string `json:"capabilities"`
}

type RegisterResponse struct {
	WorkerID string `json:"worker_id"`
	Name     string `json:"name"`
	State    string `json:"state"`
}

func (c *Client) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/workers", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("register: unexpected status %d", resp.StatusCode)
	}
	var out RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Heartbeat(ctx context.Context, workerID string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/workers/"+workerID+"/heartbeat", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("heartbeat: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// ClaimedJob is the response from a successful claim.
type ClaimedJob struct {
	JobID          string `json:"job_id"`
	AttemptID      string `json:"attempt_id"`
	AttemptNum     int    `json:"attempt_num"`
	Kind           string `json:"kind"`
	Payload        []byte `json:"payload"`
	MaxAttempts    int    `json:"max_attempts"`
	TimeoutSecs    int    `json:"timeout_secs"`
	LeaseToken     string `json:"lease_token"`
	LeaseExpiresAt string `json:"lease_expires_at"`
	CorrelationID  string `json:"correlation_id,omitempty"`
}

// Claim polls for the next available job. Returns nil, nil when the queue is empty (HTTP 204).
func (c *Client) Claim(ctx context.Context, workerID string) (*ClaimedJob, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/workers/"+workerID+"/claim", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil, nil
	case http.StatusOK:
		var job ClaimedJob
		if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
			return nil, fmt.Errorf("claim: decode response: %w", err)
		}
		return &job, nil
	default:
		return nil, fmt.Errorf("claim: unexpected status %d", resp.StatusCode)
	}
}

func (c *Client) Offline(ctx context.Context, workerID string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/workers/"+workerID, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("offline: unexpected status %d", resp.StatusCode)
	}
	return nil
}
