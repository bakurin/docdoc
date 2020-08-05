package main

import (
	"context"
	"fmt"
	"github.com/newrelic/newrelic-telemetry-sdk-go/telemetry"
	"github.com/sirupsen/logrus"
	"io"
	"time"
)

//Harvester is an interface which describes a StatusEvent consumer
type Harvester interface {
	Harvest(ctx context.Context, src <-chan StatusEvent) error
}

// FuncHarvester is Harvester implementation for testing
type FuncHarvester func(ctx context.Context, src <-chan StatusEvent) error

func (fn FuncHarvester) Harvest(ctx context.Context, src <-chan StatusEvent) error {
	return fn(ctx, src)
}

// IOWriterHarvester implements Harvester interface and logs StatusEvents with provided Writer
type IOWriterHarvester struct {
	writer io.Writer
}

func (h IOWriterHarvester) Harvest(ctx context.Context, src <-chan StatusEvent) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-src:
			_, _ = h.writer.Write([]byte(fmt.Sprintf("event recorded: \"%v\"\n", e)))
		}
	}
}

// NewrelicHarvester is options to configure New Relic client
type NewrelicOptions struct {
	ApiKey          string
	ApplicationName string
	HostName        string
	Environment     string
}

// NewrelicHarvester is New Relic client based implementation of Harvester
type NewrelicHarvester struct {
	metricName string
	harvester  *telemetry.Harvester
	logger     *logrus.Logger
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
