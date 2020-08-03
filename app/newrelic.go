package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"
)

type newrelic struct {
	client http.Client
}

func NewNewrelic() Harvester {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &newrelic{
		client: http.Client{
			Transport: transport,
		},
	}
}

func (nr newrelic) Harvest(ctx context.Context, src <-chan StatusEvent) error {
	payload, err := json.Marshal(map[string]string{
		"key": "value",
	})

	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "", bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	res, err := nr.client.Do(req)
	if err != nil {
		return err
	}

	defer res.Body.Close()

	return nil
}
