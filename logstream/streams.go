// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package logstream

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/oracle/oracle-db-appdev-monitoring/config"
	"github.com/oracle/oracle-db-appdev-monitoring/db"
	"github.com/oracle/oracle-db-appdev-monitoring/otlp"
	"github.com/oracle/oracle-db-appdev-monitoring/querylog"
	"github.com/oracle/oracle-db-appdev-monitoring/restart"
)

// Start runs configured log streams and reloads their definition files until
// ctx is canceled. All workers are tracked by the caller's wait group.
func Start(ctx context.Context, workers *sync.WaitGroup, logger *slog.Logger, m *config.MetricsConfiguration, databases []*db.Database, version string) error {
	if len(databases) == 0 {
		return nil
	}
	watchCtx, cancelWatching := context.WithCancel(ctx)
	reloads := make(chan struct{}, 1)
	if err := watchFiles(watchCtx, logger, m, func() {
		select {
		case reloads <- struct{}{}:
		default:
		}
	}); err != nil {
		cancelWatching()
		return err
	}
	// Read again after installing watchers so an edit during startup cannot
	// leave collectors using definitions older than the watched fingerprint.
	updated := *m
	var err error
	updated.LogStreams, err = config.LoadLogStreams(m)
	if err != nil {
		cancelWatching()
		return err
	}
	collectors, err := newCollectors(&updated, databases, version)
	if err != nil {
		cancelWatching()
		return err
	}
	logger.Info("Configured database log streams", "streams", len(collectors.collectors), "state_directory", m.Logging.StateDirectory)
	workers.Go(func() {
		defer cancelWatching()
		stop := runCollectors(ctx, logger, collectors)
		defer func() { stop() }()
		for {
			select {
			case <-ctx.Done():
				return
			case <-reloads:
				updated.LogStreams, err = config.LoadLogStreams(m)
				if err != nil {
					logger.Error("Unable to reload log streams; keeping current collectors", "error", err)
					continue
				}
				next, err := newCollectors(&updated, databases, version)
				if err != nil {
					logger.Error("Unable to configure reloaded log streams; keeping current collectors", "error", err)
					continue
				}
				// Release sessions, outputs and checkpoint locks before replacements
				// can poll the same database/stream and resume its saved progress.
				stop()
				stop = runCollectors(ctx, logger, next)
				logger.Info("Reloaded database log streams", "streams", len(next.collectors))
			}
		}
	})
	return nil
}

func watchFiles(ctx context.Context, logger *slog.Logger, m *config.MetricsConfiguration, changed func()) error {
	seen := make(map[string]bool)
	for _, path := range m.LogStreamDefinitionFiles() {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		if err := restart.WatchConfigFile(ctx, logger, path, func() error {
			_, err := config.LoadLogStreams(m)
			return err
		}, changed); err != nil {
			return fmt.Errorf("watch log stream definitions %q: %w", path, err)
		}
	}
	return nil
}

// runCollectors returns a drain function owned by the reload coordinator.
func runCollectors(ctx context.Context, logger *slog.Logger, group *collectorGroup) func() {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	for _, c := range group.collectors {
		workers.Go(func() {
			defer func() {
				if err := c.Close(); err != nil {
					logger.Error("Close database log collector", "error", err)
				}
			}()
			c.Run(ctx, logger)
		})
	}
	return func() {
		cancel()
		workers.Wait()
		if group.transport != nil {
			if err := group.transport.Close(); err != nil {
				logger.Error("Close OTLP log transport", "error", err)
			}
		}
	}
}

type collectorGroup struct {
	collectors []*querylog.Collector
	transport  *otlp.LogTransport
}

func newCollectors(m *config.MetricsConfiguration, databases []*db.Database, version string) (*collectorGroup, error) {
	group := &collectorGroup{}
	cleanup := func() {
		for _, c := range group.collectors {
			_ = c.Close()
		}
		if group.transport != nil {
			_ = group.transport.Close()
		}
	}
	if len(databases) == 0 || len(m.LogStreams) == 0 {
		return group, nil
	}
	makeOutput := func(out config.LogOutput, database, timestampColumn string) (querylog.Output, error) {
		switch out.Type {
		case "otlp":
			if group.transport == nil {
				var err error
				group.transport, err = otlp.NewLogTransport(m.OTLP, version)
				if err != nil {
					return nil, err
				}
			}
			return group.transport.Output(timestampColumn), nil
		case "stdout":
			return querylog.NewFileOutput("stdout"), nil
		default:
			path, err := querylog.Destination(out.Destination, database, out.PerDatabaseFiles)
			if err != nil {
				return nil, err
			}
			return querylog.NewFileOutput(path), nil
		}
	}
	add := func(database *db.Database, s config.LogStreamConfig) error {
		output, err := makeOutput(s.Output, database.Name, s.TimestampColumn)
		if err != nil {
			return err
		}
		opts := querylog.Options{TimestampColumn: s.TimestampColumn, Database: database.Name, Stream: s.ID, StateDirectory: m.Logging.StateDirectory, Interval: s.Interval, QueryTimeout: s.QueryTimeout, InitialLookback: s.InitialLookback, MaxRows: s.MaxRows, MaxBytes: s.MaxBytes}
		c, err := querylog.New(opts, &querylog.SQLSource{Database: database, Query: s.Query}, output)
		if err != nil {
			_ = output.Close()
			return err
		}
		group.collectors = append(group.collectors, c)
		return nil
	}
	for _, database := range databases {
		for _, s := range m.LogStreams {
			selected := len(s.Databases) == 0
			for _, name := range s.Databases {
				if name == database.Name {
					selected = true
				}
			}
			if selected {
				if err := add(database, s); err != nil {
					cleanup()
					return nil, err
				}
			}
		}
	}
	return group, nil
}
