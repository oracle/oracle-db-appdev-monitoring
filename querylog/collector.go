// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package querylog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	collected    = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "oracledb_exporter_log_records_total", Help: "Acknowledged database log records, including possible crash replay."}, []string{"database", "stream"})
	failures     = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "oracledb_exporter_log_errors_total", Help: "Database log collection failures."}, []string{"database", "stream"})
	cursorMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "oracledb_exporter_log_cursor_timestamp_seconds", Help: "Saved database log timestamp cursor."}, []string{"database", "stream"})
)

func init() { prometheus.MustRegister(collected, failures, cursorMetric) }

type Options struct {
	Database, Stream, StateDirectory, TimestampColumn string
	Interval, QueryTimeout, InitialLookback           time.Duration
	MaxRows, MaxBytes                                 int
	BatchRows                                         int
}

type Collector struct {
	opts    Options
	source  Source
	output  Output
	mu      sync.Mutex
	file    *stateFile
	state   checkpoint
	pending *checkpoint
	save    func(checkpoint) error
}

func New(opts Options, source Source, output Output) (*Collector, error) {
	if opts.TimestampColumn == "" || opts.Database == "" || opts.Stream == "" || opts.StateDirectory == "" || opts.Interval <= 0 || opts.QueryTimeout <= 0 || opts.InitialLookback < 0 || opts.MaxRows < 1 || opts.MaxBytes < 1024 {
		return nil, errors.New("invalid log collector options")
	}
	if opts.BatchRows == 0 {
		opts.BatchRows = 256
	}
	if opts.BatchRows < 1 || source == nil || output == nil {
		return nil, errors.New("source, output and positive batch size are required")
	}
	return &Collector{opts: opts, source: source, output: output}, nil
}

// Poll serializes stream progress. All reads complete and release their session
// before delivery. Dense ranges are subdivided; a single oversized timestamp
// group fails explicitly rather than skipping rows.
func (c *Collector) Poll(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending != nil {
		if err := c.save(*c.pending); err != nil {
			return fmt.Errorf("checkpoint: %w", err)
		}
		c.state = *c.pending
		c.pending = nil
	}
	if c.file == nil {
		file, state, err := openState(c.opts.StateDirectory, c.opts.Database+"\x00"+c.opts.Stream)
		if err != nil {
			return fmt.Errorf("checkpoint: %w", err)
		}
		c.file = file
		c.state = state
		c.save = file.save
	}
	readCtx, cancel := context.WithTimeout(ctx, c.opts.QueryTimeout)
	defer cancel()
	r, err := c.source.Open(readCtx)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	released := false
	defer func() {
		if !released {
			_ = r.Close()
		}
	}()
	now := r.Now().UTC()
	state := c.state
	initialize := state.CompletedThrough.IsZero()
	if initialize {
		from := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
		if c.opts.InitialLookback > 0 {
			from = now.Add(-c.opts.InitialLookback)
		}
		// Store the initial lower bound as the same exclusive cursor used after
		// completed reads, so retries retain the initial replay policy.
		state.CompletedThrough = from.Add(-time.Nanosecond)
	}
	if now.Before(state.CompletedThrough) {
		return errors.New("source clock precedes checkpoint completed timestamp; collection paused")
	}
	// Sources accept inclusive bounds. FF9 preserves this one-nanosecond step,
	// excluding the saved cursor without changing any event timestamps.
	from := state.CompletedThrough.Add(time.Nanosecond)
	if from.After(now) {
		return nil
	}
	upper := now
	var rows []Row
	for attempts := 0; ; attempts++ {
		rows, err = r.Read(readCtx, from, upper, c.opts.MaxRows, c.opts.MaxBytes)
		if !errors.Is(err, ErrLimit) {
			break
		}
		next := from.Add(upper.Sub(from) / 2)
		if attempts >= 63 || !next.Before(upper) {
			return fmt.Errorf("%w; increase maxRows/maxBytes", ErrLimit)
		}
		upper = next
	}
	closeErr := r.Close()
	released = true
	if err = errors.Join(err, closeErr); err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	cancel()
	// Validate the full range before emitting any output.
	for _, row := range rows {
		timestamp, err := row.Timestamp(c.opts.TimestampColumn)
		if err != nil {
			return err
		}
		if timestamp.Before(from) || timestamp.After(upper) {
			return errors.New("source returned a row outside its requested timestamp range")
		}
	}
	if initialize {
		if err := c.persist(state); err != nil {
			return fmt.Errorf("checkpoint initial boundary: %w", err)
		}
	}
	for start := 0; start < len(rows); start += c.opts.BatchRows {
		batch := rows[start:min(start+c.opts.BatchRows, len(rows))]
		if err := c.output.Deliver(ctx, batch); err != nil {
			return fmt.Errorf("deliver: %w", err)
		}
		collected.WithLabelValues(c.opts.Database, c.opts.Stream).Add(float64(len(batch)))
	}
	// Advance only after the whole range is acknowledged. Failed partial ranges
	// may replay delivered batches; timestamp ties never advance individually.
	state.CompletedThrough = upper
	if err := c.persist(state); err != nil {
		return fmt.Errorf("checkpoint: %w", err)
	}
	cursorMetric.WithLabelValues(c.opts.Database, c.opts.Stream).Set(float64(upper.UnixNano()) / 1e9)
	return nil
}

// persist blocks further reads and delivery if an acknowledged cursor cannot be
// saved. Retry this exact state before polling again.
func (c *Collector) persist(state checkpoint) error {
	if err := c.save(state); err != nil {
		c.pending = &state
		return err
	}
	c.state = state
	return nil
}

func (c *Collector) Run(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(c.opts.Interval)
	defer ticker.Stop()
	backoff := time.Duration(0)
	var retryAfter time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if time.Now().Before(retryAfter) {
			continue
		}
		err := c.Poll(ctx)
		if err != nil && ctx.Err() == nil && !errors.Is(err, ErrNotReady) {
			failures.WithLabelValues(c.opts.Database, c.opts.Stream).Inc()
			if backoff == 0 {
				backoff = time.Minute
			} else {
				backoff = min(backoff*2, 15*time.Minute)
			}
			retryAfter = time.Now().Add(backoff)
			logger.Error("Database log collection failed", "database", c.opts.Database, "stream", c.opts.Stream, "error", err, "retry_after", retryAfter)
		} else {
			backoff = 0
			retryAfter = time.Time{}
		}
	}
}

func (c *Collector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	if c.file != nil {
		err = c.file.close()
		c.file = nil
	}
	return errors.Join(err, c.output.Close())
}
