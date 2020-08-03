package main

import (
	"context"
	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"github.com/sirupsen/logrus"
	"time"
)

type Harvester interface {
	Harvest(ctx context.Context, src <-chan StatusEvent) error
}

type ConsoleHarvester struct {
	logger logrus.FieldLogger
}

func (cp ConsoleHarvester) Harvest(ctx context.Context, src <-chan StatusEvent) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-src:
			logrus.Debugf("event recorded: \"%v\"\n", e)
		}
	}
}

type NewrelicHarvester struct {
	metricName string
	harvester  *telemetry.Harvester
	logger     *logrus.Logger
}

type NewrelicOptions struct {
	ApiKey          string
	ApplicationName string
	HostName        string
	Environment     string
}

func NewNewrelicHarvester(opts *NewrelicOptions, metricName string, logger *logrus.Logger) (*NewrelicHarvester, error) {
	harvester, err := telemetry.NewHarvester(
		telemetry.ConfigAPIKey(opts.ApiKey),
		telemetry.ConfigCommonAttributes(map[string]interface{}{
			"app.name":  opts.ApplicationName,
			"host.name": opts.HostName,
			"env":       opts.Environment,
		}),
		telemetry.ConfigBasicErrorLogger(logger.Out),
		telemetry.ConfigBasicDebugLogger(logger.Out),
	)
	if err != nil {
		return nil, err
	}

	return &NewrelicHarvester{
		metricName: metricName,
		harvester:  harvester,
		logger:     logger,
	}, nil
}

func (nr NewrelicHarvester) Harvest(ctx context.Context, src <-chan StatusEvent) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev := <-src:
			nr.harvester.RecordMetric(telemetry.Gauge{
				Name: nr.metricName,
				Attributes: map[string]interface{}{
					"swarm.serviceName":   ev.ServiceName,
					"swarm.desiredTasks":  ev.DesiredTasks,
					"swarm.runningTasks":  ev.RunningTasks,
					"swarm.globalService": ev.IsGlobal,
				},
				Value:     float64(ev.DesiredTasks),
				Timestamp: time.Now(),
			})
		}
	}
}
