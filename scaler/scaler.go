package scaler

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/TriggerMail/buildkite-gcp-scaler/pkg/buildkite"
	"github.com/TriggerMail/buildkite-gcp-scaler/pkg/gce"
	hclog "github.com/hashicorp/go-hclog"
)

type Config struct {
	Datadog               string
	OrgSlug               string
	GCPProject            string
	GCPZone               string
	InstanceGroupName     string
	InstanceGroupTemplate string
	BuildkiteQueue        string
	BuildkiteToken        string
	Concurrency           int
	MaxRunDuration        int64
	PollInterval          *time.Duration
}

func NewAutoscaler(cfg *Config, logger hclog.Logger) (*Scaler, error) {
	client, err := gce.NewClient(logger)
	if err != nil {
		return nil, err
	}

	scaler := Scaler{
		cfg:       cfg,
		logger:    logger.Named("scaler").With("queue", cfg.BuildkiteQueue),
		buildkite: buildkite.NewClient(cfg.OrgSlug, cfg.BuildkiteToken, logger),
		gce:       client,
	}

	if cfg.Datadog != "" {
		s, err := statsd.New(cfg.Datadog)
		if err != nil {
			return nil, err
		}
		scaler.Statsd = s
	}

	return &scaler, nil
}

type Scaler struct {
	cfg *Config

	gce interface {
		LiveInstanceCount(ctx context.Context, projectID, zone, instanceGroupName string) (int64, error)
		LaunchInstanceForGroup(ctx context.Context, projectID, zone, groupName, templateName string, maxRunDuration int64) error
	}

	buildkite interface {
		GetAgentMetrics(context.Context, string) (*buildkite.AgentMetrics, error)
	}

	logger hclog.Logger

	Statsd *statsd.Client
}

func (s *Scaler) Run(ctx context.Context) error {
	ticker := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			sem := make(chan int, s.cfg.Concurrency)
			if err := s.run(ctx, &sem); err != nil {
				s.logger.Error("Autoscaling failed", "error", err)
			}
			close(sem)

			if s.cfg.PollInterval != nil {
				ticker.Reset(*s.cfg.PollInterval)
			} else {
				return nil
			}
		}
	}
}

func (s *Scaler) run(ctx context.Context, sem *chan int) error {
	metrics, err := s.buildkite.GetAgentMetrics(ctx, s.cfg.BuildkiteQueue)
	if err != nil {
		return err
	}

	s.Statsd.Gauge("buildkite-gcp-autoscaler.scheduled_jobs", float64(metrics.ScheduledJobs), []string{}, 1)
	s.Statsd.Gauge("buildkite-gcp-autoscaler.running_jobs", float64(metrics.RunningJobs), []string{}, 1)

	totalInstanceRequirement := metrics.ScheduledJobs + metrics.RunningJobs

	liveInstanceCount, err := s.gce.LiveInstanceCount(ctx, s.cfg.GCPProject, s.cfg.GCPZone, s.cfg.InstanceGroupName)
	s.Statsd.Gauge("buildkite-gcp-autoscaler.live_instance", float64(liveInstanceCount), []string{}, 1)
	if err != nil {
		return err
	}

	if liveInstanceCount >= totalInstanceRequirement {
		return nil
	}

	required := totalInstanceRequirement - liveInstanceCount

	errChan := make(chan error, 1)
	wg := new(sync.WaitGroup)
	wg.Add(int(required))

	for i := int64(0); i < required; i++ {
		go func() {
			*sem <- 1
			defer wg.Done()
			if err := s.gce.LaunchInstanceForGroup(ctx, s.cfg.GCPProject, s.cfg.GCPZone, s.cfg.InstanceGroupName, s.cfg.InstanceGroupTemplate, s.cfg.MaxRunDuration); err != nil {
				select {
				case errChan <- err:
					s.logger.Error("Failed to launch instance", "error", err)
				default:

				}
			}

			<-*sem
		}()
	}

	wg.Wait()
	close(errChan)
	return <-errChan
}
