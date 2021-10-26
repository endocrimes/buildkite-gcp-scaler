package buildkite

import (
	"context"

	"github.com/buildkite/go-buildkite/buildkite"
	hclog "github.com/hashicorp/go-hclog"
)

type Client struct {
	OrgSlug         string
	BuildkiteClient *buildkite.Client
	Logger          hclog.Logger
}

func NewClient(org string, agentToken string, logger hclog.Logger) *Client {
	bkconfig, err := buildkite.NewTokenConfig(agentToken, false)
	if err != nil {
		return nil
	}

	return &Client{
		BuildkiteClient: buildkite.NewClient(bkconfig.Client()),
		Logger:          logger.Named("bkapi"),
		OrgSlug:         org,
	}
}

type AgentMetrics struct {
	OrgSlug       string
	Queue         string
	ScheduledJobs int64
	RunningJobs   int64
}

func (c *Client) GetAgentMetrics(ctx context.Context, queue string) (*AgentMetrics, error) {
	c.Logger.Debug("Collecting agent metrics", "queue", queue)
	builds, _, err := c.BuildkiteClient.Builds.ListByOrg(c.OrgSlug, &buildkite.BuildsListOptions{})

	if err != nil {
		return nil, err
	}

	metrics := AgentMetrics{
		OrgSlug: c.OrgSlug,
		Queue:   queue,
	}

	for _, build := range builds {
		for _, job := range build.Jobs {
			if job.State != nil && *job.State == "scheduled" {
				metrics.ScheduledJobs++
			}
			if job.State != nil && *job.State == "running" {
				metrics.RunningJobs++
			}
		}
	}

	c.Logger.Debug("Retreived agent metrics", "scheduled", metrics.ScheduledJobs, "running", metrics.RunningJobs)
	return &metrics, nil
}
