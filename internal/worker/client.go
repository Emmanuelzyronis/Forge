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
