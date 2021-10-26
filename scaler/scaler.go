package scaler

import (
	"context"
	"time"

	"github.com/TriggerMail/buildkite-gcp-scaler/pkg/buildkite"
	"github.com/TriggerMail/buildkite-gcp-scaler/pkg/gce"
	hclog "github.com/hashicorp/go-hclog"
)

type Config struct {
	OrgSlug               string
	GCPProject            string
	GCPZone               string
	InstanceGroupName     string
	InstanceGroupTemplate string
	BuildkiteQueue        string
	BuildkiteToken        string

	PollInterval *time.Duration
}

type Scaler interface {
	Run(context.Context) error
}

func NewAutoscaler(cfg *Config, logger hclog.Logger) Scaler {
	client, err := gce.NewClient(logger)
	if err != nil {
		// TODO return erros rather than panicing
		panic(err)
	}

	return &scaler{
		cfg:       cfg,
		logger:    logger.Named("scaler").With("queue", cfg.BuildkiteQueue),
		buildkite: buildkite.NewClient(cfg.OrgSlug, cfg.BuildkiteToken, logger),
		gce:       client,
	}
}

type scaler struct {
	cfg *Config

	gce interface {
		LiveInstanceCount(ctx context.Context, projectID, zone, instanceGroupName string) (int64, error)
		LaunchInstanceForGroup(ctx context.Context, projectID, zone, groupName, templateName string) error
	}

	buildkite interface {
		GetAgentMetrics(context.Context, string) (*buildkite.AgentMetrics, error)
	}

	logger hclog.Logger
}

func (s *scaler) Run(ctx context.Context) error {
	ticker := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:

			if err := s.run(ctx); err != nil {
				s.logger.Error("Autoscaling failed", "error", err)
			}

			if s.cfg.PollInterval != nil {
				ticker.Reset(*s.cfg.PollInterval)
			} else {
				return nil
			}
		}
	}
}

func (s *scaler) run(ctx context.Context) error {
	metrics, err := s.buildkite.GetAgentMetrics(ctx, s.cfg.BuildkiteQueue)
	if err != nil {
		return err
	}
	totalInstanceRequirement := metrics.ScheduledJobs + metrics.RunningJobs

	liveInstanceCount, err := s.gce.LiveInstanceCount(ctx, s.cfg.GCPProject, s.cfg.GCPZone, s.cfg.InstanceGroupName)
	if err != nil {
		return err
	}

	if liveInstanceCount >= totalInstanceRequirement {
		return nil
	}

	required := totalInstanceRequirement - liveInstanceCount

	for i := int64(0); i < required; i++ {
		err := s.gce.LaunchInstanceForGroup(ctx, s.cfg.GCPProject, s.cfg.GCPZone, s.cfg.InstanceGroupName, s.cfg.InstanceGroupTemplate)
		if err != nil {
			return err
		}
	}
	return nil
}
