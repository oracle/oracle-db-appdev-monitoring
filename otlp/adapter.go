// Copyright (c) 2026, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package otlp

import (
	"math"
	"sort"
	"time"

	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// convertFamilies adapts a Prometheus collector snapshot to SDK metric data.
func convertFamilies(families []*dto.MetricFamily) []metricdata.Metrics {
	metrics := make([]metricdata.Metrics, 0, len(families))
	for _, family := range families {
		metric, ok := convertFamily(family)
		if ok {
			metrics = append(metrics, metric)
		}
	}
	return metrics
}

func convertFamily(family *dto.MetricFamily) (metricdata.Metrics, bool) {
	if family == nil || family.Name == nil {
		return metricdata.Metrics{}, false
	}
	metric := metricdata.Metrics{Name: family.GetName(), Description: family.GetHelp()}
	switch family.GetType() {
	case dto.MetricType_GAUGE:
		points := numberPoints(family.Metric, func(sample *dto.Metric) float64 { return sample.GetGauge().GetValue() })
		metric.Data = metricdata.Gauge[float64]{DataPoints: points}
	case dto.MetricType_UNTYPED:
		points := numberPoints(family.Metric, func(sample *dto.Metric) float64 { return sample.GetUntyped().GetValue() })
		metric.Data = metricdata.Gauge[float64]{DataPoints: points}
	case dto.MetricType_COUNTER:
		points := numberPoints(family.Metric, func(sample *dto.Metric) float64 { return sample.GetCounter().GetValue() })
		metric.Data = metricdata.Sum[float64]{DataPoints: points, Temporality: metricdata.CumulativeTemporality, IsMonotonic: true}
	case dto.MetricType_HISTOGRAM:
		points := make([]metricdata.HistogramDataPoint[float64], 0, len(family.Metric))
		for _, sample := range family.Metric {
			points = append(points, histogramPoint(sample))
		}
		metric.Data = metricdata.Histogram[float64]{DataPoints: points, Temporality: metricdata.CumulativeTemporality}
	default:
		return metricdata.Metrics{}, false
	}
	return metric, true
}

func numberPoints(samples []*dto.Metric, value func(*dto.Metric) float64) []metricdata.DataPoint[float64] {
	points := make([]metricdata.DataPoint[float64], 0, len(samples))
	for _, sample := range samples {
		points = append(points, numberPoint(sample, value(sample)))
	}
	return points
}

func numberPoint(sample *dto.Metric, value float64) metricdata.DataPoint[float64] {
	return metricdata.DataPoint[float64]{Attributes: labels(sample), Time: sampleTimestamp(sample), Value: value}
}

func histogramPoint(sample *dto.Metric) metricdata.HistogramDataPoint[float64] {
	histogram := sample.GetHistogram()
	point := metricdata.HistogramDataPoint[float64]{
		Attributes: labels(sample), Time: sampleTimestamp(sample), Count: histogram.GetSampleCount(), Sum: histogram.GetSampleSum(),
	}
	var previous uint64
	for _, bucket := range histogram.Bucket {
		if math.IsInf(bucket.GetUpperBound(), 1) {
			continue
		}
		point.Bounds = append(point.Bounds, bucket.GetUpperBound())
		point.BucketCounts = append(point.BucketCounts, bucket.GetCumulativeCount()-previous)
		previous = bucket.GetCumulativeCount()
	}
	point.BucketCounts = append(point.BucketCounts, point.Count-previous)
	return point
}

func labels(sample *dto.Metric) attribute.Set {
	values := make(map[string]string, len(sample.Label))
	for _, pair := range sample.Label {
		values[pair.GetName()] = pair.GetValue()
	}
	return attribute.NewSet(attributes(values)...)
}

func attributes(values map[string]string) []attribute.KeyValue {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]attribute.KeyValue, 0, len(keys))
	for _, key := range keys {
		result = append(result, attribute.String(key, values[key]))
	}
	return result
}

func sampleTimestamp(sample *dto.Metric) time.Time {
	if sample.TimestampMs != nil {
		return time.UnixMilli(sample.GetTimestampMs())
	}
	return time.Now()
}
