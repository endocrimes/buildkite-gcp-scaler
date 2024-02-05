package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/TriggerMail/buildkite-gcp-scaler/scaler"
	"github.com/genuinetools/pkg/cli"
	hclog "github.com/hashicorp/go-hclog"
)

var (
	buildkiteToken           string
	buildkiteQueue           string
	datadogHost              string
	googleCloudProject       string
	googleCloudZone          string
	googleCloudInstanceGroup string
	googleCloudTemplateName  string
	orgSlug                  string
	interval                 string
	concurrency              int
	maxRunDuration           int64
	logger                   hclog.Logger
)

type runCommand struct{}

const runHelp = `Run a single autoscaler pass.`

func (cmd *runCommand) Name() string      { return "run" }
func (cmd *runCommand) Args() string      { return "" }
func (cmd *runCommand) ShortHelp() string { return runHelp }
func (cmd *runCommand) LongHelp() string  { return runHelp }
func (cmd *runCommand) Hidden() bool      { return false }

func (cmd *runCommand) Register(fs *flag.FlagSet) {}

func (cmd *runCommand) Run(ctx context.Context, args []string) error {
	cfg := &scaler.Config{
		GCPProject:            googleCloudProject,
		GCPZone:               googleCloudZone,
		InstanceGroupName:     googleCloudInstanceGroup,
		InstanceGroupTemplate: googleCloudTemplateName,
		BuildkiteQueue:        buildkiteQueue,
		BuildkiteToken:        buildkiteToken,
		OrgSlug:               orgSlug,
		Concurrency:           concurrency,
		Datadog:               datadogHost,
		MaxRunDuration:        maxRunDuration,
	}

	if interval != "" {
		d, err := time.ParseDuration(interval)
		if err != nil {
			return fmt.Errorf("parsing duration failed: %v", err)
		}

		cfg.PollInterval = &d
	}

	s, err := scaler.NewAutoscaler(cfg, logger)
	if err != nil {
		return fmt.Errorf("could not initialize autoscaler: %v", err)
	}

	return s.Run(ctx)
}

func main() {
	p := cli.NewProgram()
	p.Name = "buildkite-gcp-scaler"
	p.Description = `A tool that autoscales Google Cloud clusters to run Buildkite jobs`

	// Setup the global flags.
	var (
		debug bool
	)
	p.FlagSet = flag.NewFlagSet("global", flag.ExitOnError)
	p.FlagSet.BoolVar(&debug, "d", false, "enable debug logging")
	p.FlagSet.StringVar(&buildkiteToken, "buildkite-token", "", "Buildkite API Token")
	p.FlagSet.StringVar(&buildkiteQueue, "buildkite-queue", "default", "Buildkite Queue Name")
	p.FlagSet.StringVar(&googleCloudInstanceGroup, "instance-group", "", "Google Cloud Instance Group")
	p.FlagSet.StringVar(&googleCloudTemplateName, "instance-template", "", "Google Cloud Instance Template")
	p.FlagSet.StringVar(&googleCloudProject, "gcp-project", "", "Google Cloud Project")
	p.FlagSet.StringVar(&googleCloudZone, "gcp-zone", "", "Google Cloud Zone")
	p.FlagSet.StringVar(&orgSlug, "org", "", "organization slug")
	p.FlagSet.StringVar(&interval, "interval", "", "How frequently the scaler should run")
	p.FlagSet.StringVar(&datadogHost, "datadog", "", "datadog host:port")
	p.FlagSet.IntVar(&concurrency, "concurrency", 10, "How many concurrent instances to create")
	p.FlagSet.Int64Var(&maxRunDuration, "maxRunDuration", 3600, "maximum time an instance can run")

	p.Before = func(ctx context.Context) error {
		logLevel := "INFO"
		if debug {
			logLevel = "DEBUG"
		}

		logger = hclog.New(&hclog.LoggerOptions{
			Name:  "buildkite-gce-scaler",
			Level: hclog.LevelFromString(logLevel),
		})
		return nil
	}

	// Add our commands.
	p.Commands = []cli.Command{
		&runCommand{},
	}

	// Run our program.
	p.Run()
}
