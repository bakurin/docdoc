package main

import (
	"context"
	"errors"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"time"
)

type Collector interface {
	// todo: return `heartbeat` channel
	Collect(ctx context.Context, dest chan<- StatusEvent, interval time.Duration) error
}

type FuncCollector func(ctx context.Context, dest chan<- StatusEvent, interval time.Duration) error

func (fn FuncCollector) Collect(ctx context.Context, dest chan<- StatusEvent, interval time.Duration) error {
	return fn(ctx, dest, interval)
}

type DockerCollector struct {
	client *client.Client
}

func NewDockerCollector() (*DockerCollector, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		panic(err)
	}

	info, err := cli.Info(context.Background())
	if err != nil {
		return nil, err
	}

	if !info.Swarm.ControlAvailable {
		return nil, errors.New("this node is not a swarm manager")
	}

	return &DockerCollector{
		client: cli,
	}, nil
}

func (d DockerCollector) Collect(ctx context.Context, dest chan<- StatusEvent, interval time.Duration) error {
	for {
		select {
		case <-time.After(interval):
			// time limit to collect Docker Swarm metrics
			dockerCtx, _ := context.WithTimeout(ctx, 2*time.Second)
			events, err := d.listServicesStatuses(dockerCtx)
			if err != nil {
				return err
			}
			for _, e := range events {
				dest <- *e
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (d DockerCollector) listServicesStatuses(ctx context.Context) ([]*StatusEvent, error) {
	statuses := make(map[string]*StatusEvent)

	services, err := d.client.ServiceList(ctx, types.ServiceListOptions{})
	for _, s := range services {
		statuses[s.ID] = &StatusEvent{
			ServiceName:  s.Spec.Name,
			DesiredTasks: *s.Spec.Mode.Replicated.Replicas,
			IsGlobal:     s.Spec.Mode.Global != nil,
			Timestamp:    time.Now(),
		}
	}

	activeNodes, err := d.getActiveNodes(ctx)
	if err != nil {
		return nil, err
	}

	tasks, err := d.client.TaskList(ctx, types.TaskListOptions{})
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, nil
	}

	for _, task := range tasks {
		if _, ok := statuses[task.ServiceID]; !ok {
			continue
		}

		if _, nodeActive := activeNodes[task.NodeID]; nodeActive && task.Status.State == swarm.TaskStateRunning {
			statuses[task.ServiceID].RunningTasks++
		}
	}

	var res []*StatusEvent
	for _, ev := range statuses {
		res = append(res, ev)
	}

	return res, nil
}

func (d DockerCollector) getActiveNodes(ctx context.Context) (map[string]struct{}, error) {
	nodes, err := d.client.NodeList(ctx, types.NodeListOptions{})
	if err != nil {
		return nil, err
	}
	activeNodes := make(map[string]struct{})
	for _, n := range nodes {
		if n.Status.State != swarm.NodeStateDown {
			activeNodes[n.ID] = struct{}{}
		}
	}
	return activeNodes, nil
}
