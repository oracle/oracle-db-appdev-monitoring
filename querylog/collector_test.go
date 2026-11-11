// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package querylog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

type fixtureSource struct {
	events []Row
	now    time.Time
	active int
	opens  int
	fail   error
}
type fixtureReader struct {
	s      *fixtureSource
	closed bool
}

func (s *fixtureSource) Open(ctx context.Context) (Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.active++
	s.opens++
	return &fixtureReader{s: s}, nil
}
func (r *fixtureReader) Now() time.Time { return r.s.now }
func (r *fixtureReader) Close() error {
	if !r.closed {
		r.s.active--
		r.closed = true
	}
	return nil
}
func (r *fixtureReader) Read(ctx context.Context, from, to time.Time, rows, bytes int) ([]Row, error) {
	if r.s.fail != nil {
		return nil, r.s.fail
	}
	out := []Row{}
	for _, e := range r.s.events {
		timestamp, err := e.Timestamp("ts")
		if err != nil {
			return nil, err
		}
		if !timestamp.Before(from) && !timestamp.After(to) {
			out = append(out, e)
		}
	}
	if len(out) > rows {
		return nil, ErrLimit
	}
	return out, nil
}

type fixtureOutput struct {
	events []Row
	s      *fixtureSource
	fail   error
	calls  int
	failAt int
}

func (o *fixtureOutput) Deliver(ctx context.Context, events []Row) error {
	if o.s.active != 0 {
		return errors.New("session held during output")
	}
	o.calls++
	if o.fail != nil && (o.failAt == 0 || o.calls == o.failAt) {
		return o.fail
	}
	o.events = append(o.events, events...)
	return ctx.Err()
}
func (o *fixtureOutput) Close() error { return nil }
func setupCollector(t *testing.T) (*Collector, *fixtureSource, *fixtureOutput) {
	t.Helper()
	s := &fixtureSource{now: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)}
	o := &fixtureOutput{s: s}
	c, err := New(Options{TimestampColumn: "ts", Database: "db", Stream: "alert", StateDirectory: t.TempDir(), Interval: time.Second, QueryTimeout: time.Second, InitialLookback: time.Hour, MaxRows: 10000, MaxBytes: 1 << 20, BatchRows: 2}, s, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c, s, o
}
func eventAt(s *fixtureSource, d time.Duration, message string) Row {
	return Row{"ts": s.now.Add(d), "message": message}
}
func poll(t *testing.T, c *Collector) {
	t.Helper()
	if err := c.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTimestampCursorTiesLateRecordsAndRestart(t *testing.T) {
	c, s, o := setupCollector(t)
	e := eventAt(s, 0, "same")
	s.events = []Row{eventAt(s, -time.Second, "earlier"), e, e}
	poll(t, c)
	if len(o.events) != 3 {
		t.Fatalf("timestamp ties lost: %d", len(o.events))
	}
	// Late visibility at or before the saved cursor is intentionally ignored.
	s.events = append(s.events, eventAt(s, -time.Second, "late"), eventAt(s, 0, "late tie"), eventAt(s, time.Nanosecond, "next"))
	s.now = s.now.Add(time.Second)
	poll(t, c)
	if len(o.events) != 4 || o.events[3]["message"] != "next" {
		t.Fatalf("unexpected cursor delivery: %+v", o.events)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := New(c.opts, s, o)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	poll(t, restored)
	if len(o.events) != 4 {
		t.Fatal("restart replayed completed range")
	}
}

func TestPartialDeliveryReplaysRangeWithoutSkippingTies(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprintf("restart=%v", restart), func(t *testing.T) {
			c, s, o := setupCollector(t)
			for i := 0; i < 5; i++ {
				s.events = append(s.events, eventAt(s, -time.Second, fmt.Sprint(i)))
			}
			o.fail = errors.New("disk full")
			o.failAt = 2
			if err := c.Poll(context.Background()); !errors.Is(err, o.fail) {
				t.Fatalf("expected delivery failure: %v", err)
			}
			initial := s.now.Add(-c.opts.InitialLookback).Add(-time.Nanosecond)
			if !c.state.CompletedThrough.Equal(initial) || len(o.events) != 2 {
				t.Fatalf("advanced incomplete range: %+v", c.state)
			}
			if restart {
				if err := c.Close(); err != nil {
					t.Fatal(err)
				}
				var err error
				c, err = New(c.opts, s, o)
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
			}
			// A moving initial lookback would now skip the original records.
			s.now = s.now.Add(2 * time.Hour)
			o.fail = nil
			poll(t, c)
			if len(o.events) != 7 {
				t.Fatalf("expected two replays plus five rows: %d", len(o.events))
			}
			for i, event := range o.events[2:] {
				if event["message"] != fmt.Sprint(i) {
					t.Fatalf("timestamp tie skipped: %+v", o.events)
				}
			}
			if !c.state.CompletedThrough.Equal(s.now) {
				t.Fatal("completed range did not advance")
			}
			poll(t, c)
			if len(o.events) != 7 {
				t.Fatal("completed range replayed")
			}
		})
	}
}

func TestCompletedPollWritesOneSmallCheckpoint(t *testing.T) {
	c, s, _ := setupCollector(t)
	poll(t, c)
	s.now = s.now.Add(time.Second)
	for i := 0; i < 10000; i++ {
		s.events = append(s.events, eventAt(s, 0, fmt.Sprint(i)))
	}
	save := c.save
	writes := 0
	c.save = func(state checkpoint) error {
		writes++
		return save(state)
	}
	poll(t, c)
	if writes != 1 {
		t.Fatalf("checkpoint writes for one poll: %d", writes)
	}
	data, err := os.ReadFile(c.file.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 128 {
		t.Fatalf("checkpoint grew with row count: %d bytes", len(data))
	}
}

func TestCheckpointFailurePausesReadsAndRetriesPersistence(t *testing.T) {
	c, s, o := setupCollector(t)
	poll(t, c)
	s.now = s.now.Add(2 * time.Second)
	s.events = []Row{eventAt(s, -time.Second, "new")}
	save := c.save
	c.save = func(checkpoint) error { return errors.New("sync failed") }
	if err := c.Poll(context.Background()); err == nil {
		t.Fatal("expected persistence failure")
	}
	opens := s.opens
	if err := c.Poll(context.Background()); err == nil {
		t.Fatal("expected persistence failure")
	}
	if s.opens != opens {
		t.Fatal("read continued after checkpoint failure")
	}
	c.save = save
	poll(t, c)
	if len(o.events) != 1 {
		t.Fatal("redelivered pending batch")
	}
}
func TestCrashAfterDeliveryMayReplay(t *testing.T) {
	c, s, o := setupCollector(t)
	poll(t, c)
	s.now = s.now.Add(2 * time.Second)
	s.events = []Row{eventAt(s, -time.Second, "new")}
	c.save = func(checkpoint) error { return errors.New("crash before checkpoint") }
	if err := c.Poll(context.Background()); err == nil {
		t.Fatal("expected failure")
	}
	_ = c.Close()
	restored, err := New(c.opts, s, o)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	poll(t, restored)
	if len(o.events) != 2 {
		t.Fatalf("expected at-least-once replay, got %d", len(o.events))
	}
}
func TestDenseRangeSubdivisionAndOversizedTie(t *testing.T) {
	c, s, o := setupCollector(t)
	c.opts.MaxRows = 3
	for i := 0; i < 8; i++ {
		s.events = append(s.events, eventAt(s, -time.Duration(50-i*5)*time.Minute, fmt.Sprint(i)))
	}
	for i := 0; i < 20 && len(o.events) < 8; i++ {
		poll(t, c)
	}
	if len(o.events) != 8 {
		t.Fatalf("catchup starved: %d", len(o.events))
	}
	s.now = s.now.Add(2 * time.Second)
	s.events = nil
	for i := 0; i < 4; i++ {
		s.events = append(s.events, eventAt(s, -time.Second, fmt.Sprint(i)))
	}
	for attempt := 0; attempt < 100; attempt++ {
		err := c.Poll(context.Background())
		if errors.Is(err, ErrLimit) {
			if len(o.events) != 8 {
				t.Fatal("oversized timestamp was partially delivered")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("oversized timestamp did not fail explicitly")
}
func TestOutageQuietSourceAndClockRegression(t *testing.T) {
	c, s, o := setupCollector(t)
	poll(t, c)
	s.events = []Row{eventAt(s, -30*time.Second, "late after quiet poll"), eventAt(s, time.Hour, "during outage")}
	s.now = s.now.Add(2 * time.Hour)
	poll(t, c)
	if len(o.events) != 1 || o.events[0]["message"] != "during outage" {
		t.Fatal("outage skipped new records or replayed late records")
	}
	s.now = s.now.Add(-time.Hour)
	if err := c.Poll(context.Background()); err == nil {
		t.Fatal("clock regression was ignored")
	}
}
func TestNewSourceResumesDatabaseAliasCheckpoint(t *testing.T) {
	c, s, o := setupCollector(t)
	s.events = []Row{eventAt(s, -time.Second, "A")}
	poll(t, c)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	replacement := &fixtureSource{now: s.now.Add(time.Second), events: append([]Row(nil), s.events...)}
	replacement.events = append(replacement.events, eventAt(replacement, 0, "B"))
	output := &fixtureOutput{s: replacement}
	resumed, err := New(c.opts, replacement, output)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	poll(t, resumed)
	if len(o.events) != 1 || len(output.events) != 1 || output.events[0]["message"] != "B" {
		t.Fatalf("new connection replayed old records: %+v", output.events)
	}
}

func TestCheckpointOwnershipAndCorruption(t *testing.T) {
	dir := t.TempDir()
	f, state, err := openState(dir, "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := openState(dir, "key"); err == nil {
		t.Fatal("shared checkpoint lock")
	}
	state.CompletedThrough = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	if err := f.save(state); err != nil {
		t.Fatal(err)
	}
	_ = f.close()
	restored, resumed, err := openState(dir, "key")
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.CompletedThrough.Equal(state.CompletedThrough) {
		t.Fatal("checkpoint progress was lost")
	}
	_ = restored.close()
	for _, data := range []string{
		"{broken",
		`{"version":1,"frontier":"2026-11-11T12:00:00Z"}`,
		`{"version":99,"completedThrough":"2026-11-11T12:00:00Z"}`,
		`{"version":2}`,
		`null`,
	} {
		if err := os.WriteFile(f.path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := openState(dir, "key"); err == nil {
			t.Fatalf("invalid checkpoint accepted: %s", data)
		}
	}
}
func TestConcurrentPollSerializesProgress(t *testing.T) {
	c, s, o := setupCollector(t)
	s.events = []Row{eventAt(s, -time.Second, "new")}
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Poll(context.Background()); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(o.events) != 1 {
		t.Fatal("overlapping polls replayed records")
	}
}
func TestRejectedOutputRetriesWithoutAdvancingCheckpoint(t *testing.T) {
	c, s, o := setupCollector(t)
	s.events = []Row{eventAt(s, -time.Second, "new")}
	o.fail = ErrRejected
	for i := 0; i < 2; i++ {
		if err := c.Poll(context.Background()); !errors.Is(err, ErrRejected) {
			t.Fatal(err)
		}
		if !c.state.CompletedThrough.Before(s.events[0]["ts"].(time.Time)) {
			t.Fatal("rejection advanced checkpoint past undelivered record")
		}
	}
	opens := s.opens
	o.fail = nil
	poll(t, c)
	if s.opens != opens+1 || len(o.events) != 1 || !c.state.CompletedThrough.Equal(s.now) {
		t.Fatal("rejected stream did not recover and checkpoint delivery")
	}
	poll(t, c)
	if len(o.events) != 1 {
		t.Fatal("replayed acknowledged records")
	}
}
func TestCancellationReleasesSource(t *testing.T) {
	c, s, _ := setupCollector(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.Poll(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if s.active != 0 {
		t.Fatal("connection leaked")
	}
}

func TestCheckpointSyncNeverHoldsSourceSession(t *testing.T) {
	c, s, _ := setupCollector(t)
	poll(t, c)
	save := c.save
	c.save = func(state checkpoint) error {
		if s.active != 0 {
			return errors.New("session held across checkpoint sync")
		}
		return save(state)
	}
	s.now = s.now.Add(2 * time.Second)
	s.events = []Row{eventAt(s, -time.Second, "new")}
	poll(t, c)
}

func TestCheckpointFormat(t *testing.T) {
	file, state, err := openState(t.TempDir(), "key")
	if err != nil {
		t.Fatal(err)
	}
	state.CompletedThrough = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	if err := file.save(state); err != nil {
		t.Fatal(err)
	}
	if err := file.close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document) != 2 || document["version"] == nil || document["completedThrough"] == nil {
		t.Fatalf("unexpected checkpoint fields: %s", data)
	}
}

func TestCheckpointStorageFailureRetriesWithoutDelivering(t *testing.T) {
	c, s, o := setupCollector(t)
	parent := t.TempDir() + "/state"
	if err := os.WriteFile(parent, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	c.opts.StateDirectory = parent + "/checkpoints"
	s.events = []Row{eventAt(s, -time.Second, "A")}
	if err := c.Poll(context.Background()); err == nil {
		t.Fatal("checkpoint failure ignored")
	}
	if s.opens != 0 || s.active != 0 || len(o.events) != 0 {
		t.Fatal("checkpoint failure opened a SQL source or delivered rows")
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	poll(t, c)
	if len(o.events) != 1 {
		t.Fatal("collection did not recover when storage became available")
	}
}
