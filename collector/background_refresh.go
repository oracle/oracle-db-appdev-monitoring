// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import (
	"context"
	"time"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
)

// refreshesInBackground reports whether a due metric is refreshed in the background instead of
// during the scrape. Only metrics with a custom scrape interval are, as their cached values are
// already served between refreshes.
func (e *Exporter) refreshesInBackground(metric *config.Metric) bool {
	if e.MetricsConfiguration == nil || !e.BackgroundRefreshEnabled() {
		return false
	}
	return hasCustomScrapeInterval(metric)
}

// startBackgroundRefresh refreshes a metric in the background, unless a refresh of it is already
// running or background refreshes have been stopped. The refreshed values replace the cached values
// once the query completes, and are served from then on.
func (e *Exporter) startBackgroundRefresh(d *db.Database, cache *MetricsCache, generation uint64, metric *config.Metric, tick *time.Time) {
	ctx, slots, ok := e.beginRefresh(d)
	if !ok {
		return
	}
	if !cache.StartRefresh(metric) {
		// The previous refresh of this metric is still running.
		e.refreshes.Done()
		return
	}
	e.logger.Debug("Refreshing metric in the background", "Context", metric.Context, "database", d.Name)
	go func() {
		defer e.refreshes.Done()
		if slots != nil {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				cache.FinishRefresh(metric, tick, ctx.Err())
				return
			}
		}
		err := e.scrapeAndRecordMetric(ctx, d, cache, generation, nil, metric)
		cache.FinishRefresh(metric, tick, err)
	}()
}

// beginRefresh registers a new background refresh, and returns the context it must run with and the
// slots limiting the concurrent refreshes of the database. It returns false once background
// refreshes have been stopped.
func (e *Exporter) beginRefresh(d *db.Database) (context.Context, chan struct{}, bool) {
	e.refreshMu.Lock()
	defer e.refreshMu.Unlock()
	if e.refreshStopped {
		return nil, nil, false
	}
	if e.refreshCtx == nil {
		e.refreshCtx, e.cancelRefreshes = context.WithCancel(context.Background())
	}
	e.refreshes.Add(1)
	return e.refreshCtx, e.refreshSlotsLocked(d), true
}

// refreshSlotsLocked returns the slots limiting the concurrent background refreshes of a database.
// Background refreshes may use all but one of the database's connections, so a scrape can always
// ping the database and run its other queries. Callers must hold refreshMu.
func (e *Exporter) refreshSlotsLocked(d *db.Database) chan struct{} {
	if slots, ok := e.refreshSlots[d]; ok {
		return slots
	}
	if e.refreshSlots == nil {
		e.refreshSlots = map[*db.Database]chan struct{}{}
	}
	var slots chan struct{}
	if maxOpen := d.MaxOpenConnections(); maxOpen > 0 {
		slots = make(chan struct{}, max(maxOpen-1, 1))
	}
	e.refreshSlots[d] = slots
	return slots
}

// StopRefreshes stops starting background refreshes, cancels the running ones, and waits until
// they have finished or ctx is done. Call it before closing the databases.
func (e *Exporter) StopRefreshes(ctx context.Context) error {
	e.refreshMu.Lock()
	e.refreshStopped = true
	if e.cancelRefreshes != nil {
		e.cancelRefreshes()
	}
	e.refreshMu.Unlock()

	done := make(chan struct{})
	go func() {
		e.refreshes.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *Exporter) currentCacheGeneration() uint64 {
	e.generationMu.Lock()
	defer e.generationMu.Unlock()
	return e.cacheGeneration
}

// observeCurrentMetricScrapeDuration records the scrape duration of a metric, unless the metrics
// were reloaded since the scrape started: the reload resets the durations, and the metric may no
// longer exist.
func (e *Exporter) observeCurrentMetricScrapeDuration(generation uint64, d *db.Database, m *config.Metric, result string, elapsed time.Duration) {
	e.generationMu.Lock()
	defer e.generationMu.Unlock()
	if generation != e.cacheGeneration {
		return
	}
	e.observeMetricScrapeDuration(d, m, result, elapsed)
}
