package main

import (
	"context"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func createNullLogger() *logrus.Logger {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetFormatter(&logrus.TextFormatter{
		DisableTimestamp: true,
	})
	log.SetOutput(ioutil.Discard)
	return log
}

func TestRun_CancelOnHarvesterTerminated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := FuncCollector(func(ctx context.Context, dest chan<- StatusEvent, interval time.Duration) <-chan struct{} {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		heartbeat := make(chan struct{}, 1)
		go func() {
			defer close(heartbeat)
			for {
				select {
				case <-ticker.C:
					heartbeat <- struct{}{}
				case <-ctx.Done():
					return
				}
			}
		}()

		return heartbeat
	})

	harvester := FuncHarvester(func(ctx context.Context, src <-chan StatusEvent) error {
		return nil
	})

	go func() {
		run(collector, harvester, ctx, time.Second, createNullLogger())
		cancel()
	}()

	select {
	case <-time.After(time.Second):
		t.Errorf("program supposet to be cancelled once harvester is stopped")
	case <-ctx.Done():
		return
	}
}

func TestRun_CancelOnCollectorTerminated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	collector := FuncCollector(func(ctx context.Context, dest chan<- StatusEvent, interval time.Duration) <-chan struct{} {
		heartbeat := make(chan struct{}, 1)
		defer close(heartbeat)
		return heartbeat
	})

	harvester := FuncHarvester(func(ctx context.Context, src <-chan StatusEvent) error {
		for {
			select {
			case <-src: // just listen the source and ignore context
			case <-ctx.Done():
				return nil
			}
		}
	})

	go func() {
		run(collector, harvester, ctx, time.Second, createNullLogger())
		cancel()
	}()

	select {
	case <-time.After(time.Second):
		assert.Fail(t, "program supposed to be cancelled once collector is stopped")
	case <-ctx.Done():
		return
	}
}

func TestRun_CancelWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	collectorCtx, _ := context.WithCancel(ctx)
	collector := FuncCollector(func(ctx context.Context, dest chan<- StatusEvent, interval time.Duration) <-chan struct{} {
		heartbeat := make(chan struct{}, 1)
		go func() {
			defer close(heartbeat)
			select {
			case <-ctx.Done():
				collectorCtx.Done()
				return
			}
		}()
		return heartbeat
	})

	harvesterCtx, _ := context.WithCancel(ctx)
	harvester := FuncHarvester(func(ctx context.Context, src <-chan StatusEvent) error {
		for {
			select {
			case <-src: // just listen the source and ignore context
			case <-ctx.Done():
				harvesterCtx.Done()
				return nil
			}
		}
	})

	go func() {
		go run(collector, harvester, ctx, time.Second, createNullLogger())
		cancel()
	}()

	select {
	case <-time.After(time.Second):
		assert.Fail(t, "program supposed to be cancelled")
	case <-ctx.Done():
		assert.Equal(t, context.Canceled, collectorCtx.Err())
		assert.Equal(t, context.Canceled, harvesterCtx.Err())
	}
}

func TestRun_PassEventFromCollectorToHarvester(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	collector := FuncCollector(func(ctx context.Context, dest chan<- StatusEvent, interval time.Duration) <-chan struct{} {
		heartbeat := make(chan struct{}, 1)
		defer close(heartbeat)
		dest <- StatusEvent{} // post single event
		<-ctx.Done()
		return heartbeat
	})

	var counter int
	harvester := FuncHarvester(func(ctx context.Context, src <-chan StatusEvent) error {
		<-src // waite for event
		counter += 1
		return nil
	})

	go func() {
		run(collector, harvester, ctx, time.Second, createNullLogger())
		cancel()
	}()

	select {
	case <-time.After(time.Second):
		assert.Fail(t, "time out")
	case <-ctx.Done():
		assert.Equal(t, 1, counter, "exactly one event had to be collected and harvested")
	}
}
