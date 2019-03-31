package buildkite

// This is based on https://github.com/buildkite/buildkite-agent-scaler/blob/8bb509f42e53b07b826650b2a50e2ce6d7ca65e4/buildkite/buildkite.go

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	hclog "github.com/hashicorp/go-hclog"
)

type Client struct {
	Endpoint   string
	AgentToken string
	UserAgent  string
	HTTPClient *http.Client
	Logger     hclog.Logger
}

func NewClient(agentToken string, logger hclog.Logger) *Client {
	return &Client{
		Endpoint:   "https://agent.buildkite.com/v3",
		UserAgent:  "buildkite-gce-scaler/0.1",
		AgentToken: agentToken,
		HTTPClient: cleanhttp.DefaultClient(),
		Logger:     logger.Named("bkapi"),
	}
}

type AgentMetrics struct {
	OrgSlug       string
	Queue         string
	ScheduledJobs int64
	RunningJobs   int64
}

type metricsQueryResponse struct {
	Organization struct {
		Slug string `json:"slug"`
	} `json:"organization"`
	Jobs struct {
		Queues map[string]struct {
			Scheduled int64 `json:"scheduled"`
			Running   int64 `json:"running"`
		} `json:"queues"`
	} `json:"jobs"`
}

func (m *metricsQueryResponse) agentMetrics(queue string) *AgentMetrics {
	var metrics AgentMetrics
	metrics.OrgSlug = m.Organization.Slug
	metrics.Queue = queue

	if queue, exists := m.Jobs.Queues[queue]; exists {
		metrics.ScheduledJobs = queue.Scheduled
		metrics.RunningJobs = queue.Running
	}

	return &metrics
}

func (c *Client) GetAgentMetrics(ctx context.Context, queue string) (*AgentMetrics, error) {
	c.Logger.Debug("Collecting agent metrics", "queue", queue)

	t := time.Now()
	resp, err := c.getMetrics(ctx)
	if err != nil {
		return nil, err
	}
	d := time.Now().Sub(t)

	metrics := resp.agentMetrics(queue)

	c.Logger.Debug("Retreived agent metrics", "scheduled", metrics.ScheduledJobs, "running", metrics.RunningJobs, "duration", d)
	return metrics, nil
}

func (c *Client) getMetrics(ctx context.Context) (*metricsQueryResponse, error) {
	endpoint, err := url.Parse(c.Endpoint)
	if err != nil {
		return nil, err
	}
	endpoint.Path += "/metrics"

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", c.AgentToken))

	res, err := c.HTTPClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var response metricsQueryResponse
	return &response, json.NewDecoder(res.Body).Decode(&response)
}
