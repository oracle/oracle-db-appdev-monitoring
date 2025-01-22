// Copyright (c) 2024, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package collector

import "errors"

type zeroResultError struct {
	err string
}

func (z *zeroResultError) Error() string {
	return z.err
}

func newZeroResultError() error {
	return &zeroResultError{
		err: "no metrics found while parsing, query returned no rows",
	}
}

// shouldLogScrapeError returns true if the error is a zero result error and zero result errors are ignored.
func shouldLogScrapeError(err error, isIgnoreZeroResult bool) bool {
	return !isIgnoreZeroResult || !errors.Is(err, newZeroResultError())
}
