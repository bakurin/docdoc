package main

import (
	"context"
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Opts struct {
	Interval time.Duration `long:"interval" short:"i" env:"DOCDOC_INTERVAL" required:"true" description:"refresh interval in seconds" default:"1m"`
	AppName  string        `long:"app-name" env:"DOCDOC_APP_NAME" required:"false" description:"Application name" default:"docdoc"`
	Env      string        `long:"env" env:"DOCDOC_ENV" required:"false" description:"Environment name" default:"dev"`

	NewrelicAPIKey     string `long:"nr-api-key" env:"DOCDOC_NR_API_KEY" required:"false" description:"Newrelic API key"`
	NewrelicMetricName string `long:"nr-metric-name" env:"DOCDOC_NR_METRIC_NAME" required:"false" description:"Metric name on Newrelic.com" default:"DocdocMetric"`

	Debug bool `long:"debug" env:"DEBUG" description:"debug mode"`
}

var revision = "unknown"

func main() {
	fmt.Printf("docdoc %s\n", revision)
	var opts Opts
	p := flags.NewParser(&opts, flags.Default)
	_, err := p.Parse()
	if err != nil {
		os.Exit(1)
	}

	logger := setupLog(opts.Debug)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		logger.Printf("interrupt signal")
		cancel()
	}()

	collector, err := NewDockerCollector()
	if err != nil {
		logger.Errorf("%v", err)
		os.Exit(1)
	}

	harvester, err := NewNewrelicHarvester(&NewrelicOptions{
		ApiKey:          opts.NewrelicAPIKey,
		ApplicationName: opts.NewrelicMetricName,
		Environment:     opts.Env,
	}, opts.NewrelicMetricName, logger)
	if err != nil {
		logger.Errorf("%v", err)
		os.Exit(1)
	}

	run(collector, harvester, ctx, opts.Interval, logger)
}

func setupLog(debug bool) *logrus.Logger {
	logrus.SetLevel(logrus.InfoLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	return logrus.New()
}

func run(collector Collector, harvester Harvester, ctx context.Context, interval time.Duration, logger *logrus.Logger) {
	c, cancel := context.WithCancel(ctx)
	stream := make(chan StatusEvent)
	defer close(stream)

	go (func() {
		defer cancel()
		defer logger.Debugf("collector goroutine exited")

		updatePulse := func() <-chan time.Time {
			return time.After(interval * 5)
		}

		runCollector := func() (<-chan struct{}, context.CancelFunc) {
			c, cancelCollector := context.WithCancel(c)
			return collector.Collect(c, stream, interval), cancelCollector
		}

		pulse := updatePulse()
		heartbeat, cancelCollector := runCollector()

		<-heartbeat

	loop:
		for {
			select {
			case x, ok := <-heartbeat:
				if !ok {
					logger.Debug("collector stopped")
					return
				}
				logger.Debugf("%v -> %v", x, ok)
				logger.Debug("got pulse from collector")
				pulse = updatePulse()
			case <-pulse:
				logger.Warn("collector time out. restart collector")
				cancelCollector()
				heartbeat, cancelCollector = runCollector()
				continue loop
			}
		}
	})()

	go (func() {
		defer cancel()
		defer logger.Println("harvester goroutine exited")

		err := harvester.Harvest(c, stream)
		if err == context.Canceled {
			logger.Info("provider cancelled")
			return
		}

		if err != nil {
			logger.Error(err)
		}
	})()

	logger.Println("monitoring...")
	<-c.Done()
	logger.Println("stopped")
}
