package main

import "time"

type StatusEvent struct {
	ServiceName  string
	DesiredTasks uint64
	RunningTasks uint64
	IsGlobal     bool
	Timestamp    time.Time
}
