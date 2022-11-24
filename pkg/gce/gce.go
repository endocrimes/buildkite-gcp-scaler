package gce

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/cenkalti/backoff"
	hclog "github.com/hashicorp/go-hclog"
	multierror "github.com/hashicorp/go-multierror"
	compute "google.golang.org/api/compute/v0.beta"
)

type Client struct {
	svc    *compute.Service
	gSvc   *compute.InstanceGroupsService
	iSvc   *compute.InstancesService
	logger hclog.Logger
}

func NewClient(logger hclog.Logger) (*Client, error) {
	ctx := context.Background()
	computeService, err := compute.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate Compute Service: %v", err)
	}

	return &Client{
		svc:    computeService,
		logger: logger,
		gSvc:   compute.NewInstanceGroupsService(computeService),
		iSvc:   compute.NewInstancesService(computeService),
	}, nil
}

func (c *Client) LiveInstanceCount(ctx context.Context, projectID, zone, instanceGroupName string) (int64, error) {
	result, err := c.gSvc.ListInstances(projectID, zone, instanceGroupName, &compute.InstanceGroupsListInstancesRequest{}).
		Context(ctx).
		Do()
	if err != nil {
		return 0, err
	}

	count := int64(0)
	for _, i := range result.Items {
		if i.Status == "PROVISIONING" || i.Status == "RUNNING" {
			count++
		}
	}

	return count, nil
}

func randomHex(n int) (string, error) {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (c *Client) waitForOperationCompletion(ctx context.Context, projectID, zone string, o *compute.Operation) error {
	svc := compute.NewZoneOperationsService(c.svc)
	operation := func() error {
		req := svc.Get(projectID, zone, o.Name)
		o, err := req.Context(ctx).Do()
		if err != nil {
			return backoff.Permanent(err)
		}
		c.logger.Debug("operation status", "status", o.Status)

		if o.Error != nil {
			var oErr error
			for _, err := range o.Error.Errors {
				oErr = multierror.Append(fmt.Errorf("GCE Error %s: %s", err.Code, err.Message))
			}
			return backoff.Permanent(oErr)
		}

		if o.Status == "DONE" {
			return nil
		}

		return fmt.Errorf("operation status: %s", o.Status)
	}

	return backoff.Retry(operation, backoff.NewExponentialBackOff())
}

func (c *Client) LaunchInstanceForGroup(ctx context.Context, projectID, zone, groupName, templateName string, maxRunDuration int64) error {
	suffix, err := randomHex(3)
	if err != nil {
		return err
	}

	iName := fmt.Sprintf("%s-%s", templateName, suffix)
	instance := &compute.Instance{
		Name: iName,
		Scheduling: &compute.Scheduling{
			InstanceTerminationAction: "DELETE",
			MaxRunDuration: &compute.Duration{
				Seconds: maxRunDuration,
			},
		},
	}

	c.logger.Info("creating instance", "name", iName)

	createOp, err := c.iSvc.Insert(projectID, zone, instance).
		SourceInstanceTemplate(fmt.Sprintf("projects/%s/global/instanceTemplates/%s", projectID, templateName)).
		Context(ctx).
		Do()

	if err != nil {
		return fmt.Errorf("failed to create vm: %v", err)
	}

	// Add to the group

	req := &compute.InstanceGroupsAddInstancesRequest{
		Instances: []*compute.InstanceReference{
			{
				Instance: createOp.TargetLink,
			},
		},
	}

	ao, err := c.gSvc.AddInstances(projectID, zone, groupName, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	return c.waitForOperationCompletion(ctx, projectID, zone, ao)
}
