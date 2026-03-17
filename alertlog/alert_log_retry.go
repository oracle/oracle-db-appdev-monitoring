// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package alertlog

import (
	"time"
)

const (
	initialRetryBackoff = time.Minute
	maxRetryBackoff     = 15 * time.Minute
)

type databaseRetryState struct {
	consecutiveFailures int
	retryAfter          time.Time
}

type retryTracker struct {
	state map[string]databaseRetryState
}

var databaseRetries = retryTracker{
	state: map[string]databaseRetryState{},
}

func (t *retryTracker) shouldRetry(database string, now time.Time) (bool, time.Time) {
	state, ok := t.state[database]
	if !ok {
		return true, time.Time{}
	}
	if state.retryAfter.IsZero() {
		return true, time.Time{}
	}
	if !now.Before(state.retryAfter) {
		return true, time.Time{}
	}
	return false, state.retryAfter
}

func (t *retryTracker) recordFailure(database string, now time.Time) time.Time {
	if t.state == nil {
		t.state = map[string]databaseRetryState{}
	}
	state := t.state[database]
	state.consecutiveFailures++

	backoff := initialRetryBackoff
	for i := 1; i < state.consecutiveFailures; i++ {
		backoff *= 2
		if backoff >= maxRetryBackoff {
			backoff = maxRetryBackoff
			break
		}
	}

	state.retryAfter = now.Add(backoff)
	t.state[database] = state
	return state.retryAfter
}

func (t *retryTracker) recordSuccess(database string) {
	delete(t.state, database)
}
