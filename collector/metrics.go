// Copyright (c) 2024, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl

package collector

import (
	"github.com/go-kit/log/level"
	"strconv"
)

func (e *Exporter) parseFloat(metric, metricHelp string, row map[string]string) (float64, bool) {
	value, ok := row[metric]
	if !ok {
		return -1, ok
	}
	valueFloat, err := strconv.ParseFloat(value, 64)
	if err != nil {
		level.Error(e.logger).Log("msg", "Unable to convert current value to float (metric="+metric+
			",metricHelp="+metricHelp+",value=<"+row[metric]+">)")
		return -1, false
	}
	return valueFloat, true
}
