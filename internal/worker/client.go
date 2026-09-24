package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// ErrStaleLease is returned when the API responds 409 Conflict because the
// lease token presented by the worker no longer matches the record.
var ErrStaleLease = errors.New("stale lease")

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

// postJSON marshals body as JSON, POSTs it to url, and checks the response.
// Returns ErrStaleLease on 409 Conflict; an error for any non-200 status.
func (c *Client) postJSON(ctx context.Context, url string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return ErrStaleLease
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return nil
}

func (c *Client) StartAttempt(ctx context.Context, attemptID, leaseToken string) error {
	return c.postJSON(ctx, c.baseURL+"/attempts/"+attemptID+"/start",
		map[string]string{"lease_token": leaseToken},
	)
}

type succeedAttemptBody struct {
	LeaseToken string          `json:"lease_token"`
	Result     json.RawMessage `json:"result"`
	DurationMS int64           `json:"duration_ms"`
}

func (c *Client) SucceedAttempt(ctx context.Context, attemptID, leaseToken string, result []byte, durationMS int64) error {
	return c.postJSON(ctx, c.baseURL+"/attempts/"+attemptID+"/succeed", succeedAttemptBody{
		LeaseToken: leaseToken,
		Result:     json.RawMessage(result),
		DurationMS: durationMS,
	})
}

type failAttemptBody struct {
	LeaseToken string          `json:"lease_token"`
	Reason     string          `json:"reason"`
	Detail     json.RawMessage `json:"detail"`
}

func (c *Client) FailAttempt(ctx context.Context, attemptID, leaseToken string, reason string, detail []byte) error {
	return c.postJSON(ctx, c.baseURL+"/attempts/"+attemptID+"/fail", failAttemptBody{
		LeaseToken: leaseToken,
		Reason:     reason,
		Detail:     json.RawMessage(detail),
	})
}

func (c *Client) JobHeartbeat(ctx context.Context, jobID, leaseToken string) error {
	return c.postJSON(ctx, c.baseURL+"/jobs/"+jobID+"/heartbeat",
		map[string]string{"lease_token": leaseToken},
	)
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
