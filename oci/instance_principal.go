// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package oci

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
)

const instancePrincipalHTTPTimeout = 30 * time.Second

func newInstancePrincipalProvider() (common.ConfigurationProvider, error) {
	return auth.InstancePrincipalConfigurationProviderWithCustomClient(func(dispatcher common.HTTPRequestDispatcher) (common.HTTPRequestDispatcher, error) {
		client, ok := dispatcher.(*http.Client)
		if !ok {
			return nil, fmt.Errorf("unexpected OCI instance principal HTTP client type %T", dispatcher)
		}
		// The metadata client has no default timeout. Preserve the federation
		// client's configured timeout when there is already a positive bound.
		if client.Timeout <= 0 {
			client.Timeout = instancePrincipalHTTPTimeout
		}
		return client, nil
	})
}

type instancePrincipalProvider struct {
	create   func() (common.ConfigurationProvider, error)
	mu       sync.Mutex
	provider common.ConfigurationProvider
	pending  *instancePrincipalAttempt
}

type instancePrincipalAttempt struct {
	done     chan struct{}
	provider common.ConfigurationProvider
	err      error
}

func (p *instancePrincipalProvider) get(ctx context.Context) (common.ConfigurationProvider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	if p.provider != nil {
		provider := p.provider
		p.mu.Unlock()
		return provider, nil
	}
	attempt := p.pending
	if attempt == nil {
		attempt = &instancePrincipalAttempt{done: make(chan struct{})}
		p.pending = attempt
		// The SDK bootstrap retries without a context. Let callers cancel their
		// own wait while sharing one attempt and its eventual result.
		go p.initialize(attempt)
	}
	p.mu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-attempt.done:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return attempt.provider, attempt.err
	}
}

func (p *instancePrincipalProvider) initialize(attempt *instancePrincipalAttempt) {
	provider, err := p.create()
	if err == nil {
		// Service client constructors also validate KeyID and PrivateRSAKey.
		// Fetch the initial token/key here, where callers can stop waiting.
		if _, err = common.IsConfigurationProviderValid(provider); err != nil {
			err = fmt.Errorf("validate OCI instance principal configuration: %w", err)
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if err == nil {
		p.provider = provider
		attempt.provider = provider
	}
	attempt.err = err
	p.pending = nil
	close(attempt.done)
}
