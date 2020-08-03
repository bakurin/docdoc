package main

import (
	"context"
	"fmt"
	"github.com/jessevdk/go-flags"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"sync"
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

	err = run(&opts, logger, ctx)
	if err != nil {
		logger.Errorf("[%v", err)
		os.Exit(1)
	}
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

func run(opts *Opts, logger *logrus.Logger, ctx context.Context) error {
	client, err := NewDockerCollector()
	if err != nil {
		return err
	}

	provider, err := NewNewrelicHarvester(&NewrelicOptions{
		ApiKey:          opts.NewrelicAPIKey,
		ApplicationName: opts.NewrelicMetricName,
		Environment:     opts.Env,
	}, opts.NewrelicMetricName, logger)
	if err != nil {
		return err
	}
	stream := make(chan StatusEvent)

	var wg sync.WaitGroup
	c, cancel := context.WithCancel(ctx)

	wg.Add(1)
	go (func() {
		defer wg.Done()
		defer cancel()

		err := client.Collect(c, stream, opts.Interval)
		if err == context.Canceled {
			logger.Info("client cancelled")
			return
		}

		if err != nil {
			logger.Error(err)
		}
	})()

	wg.Add(1)
	go (func() {
		defer wg.Done()
		defer cancel()

		err := provider.Harvest(c, stream)
		if err == context.Canceled {
			logger.Info("provider cancelled")
			return
		}

		if err != nil {
			logger.Error(err)
		}
	})()

	logger.Println("monitoring...")
	wg.Wait()
	logger.Println("stopped")

	return nil
}
