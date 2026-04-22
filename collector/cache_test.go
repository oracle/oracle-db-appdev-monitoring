// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsCacheStoresAndReplaysMetrics(t *testing.T) {
	metric := &Metric{ID: "sessions_value"}
	cache := NewMetricsCache(map[string]*Metric{metric.ID: metric})

	ch := make(chan prometheus.Metric, 2)
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_metric",
		Help: "test metric",
	})
	cache.CacheAndSend(ch, metric, gauge)

	if got := len(ch); got != 1 {
		t.Fatalf("expected 1 metric sent immediately, got %d", got)
	}

	cache.SendAll(ch, metric)
	if got := len(ch); got != 2 {
		t.Fatalf("expected cached metric to be replayed, got channel len %d", got)
	}

	cache.Reset(metric)
	cache.SendAll(ch, metric)
	if got := len(ch); got != 2 {
		t.Fatalf("expected reset cache to stop replaying metrics, got channel len %d", got)
	}
}

func TestMetricsCacheTracksLastScraped(t *testing.T) {
	metric := &Metric{ID: "sessions_value"}
	cache := NewMetricsCache(map[string]*Metric{metric.ID: metric})
	now := time.Now()

	cache.SetLastScraped(metric, &now)

	got := cache.GetLastScraped(metric)
	if got == nil {
		t.Fatal("expected last scraped timestamp")
	}
	if !got.Equal(now) {
		t.Fatalf("expected %v, got %v", now, *got)
	}
}
