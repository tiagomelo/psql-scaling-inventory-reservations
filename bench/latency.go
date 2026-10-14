// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package bench

import (
	"math"
	"slices"
	"time"
)

// Latency holds latency percentiles of a benchmark run.
type Latency struct {
	P50 time.Duration
	P95 time.Duration
	P99 time.Duration
	Max time.Duration
}

// Summarize computes exact latency percentiles from the observed durations.
func Summarize(durations []time.Duration) Latency {
	if len(durations) == 0 {
		return Latency{}
	}
	sorted := slices.Clone(durations)
	slices.Sort(sorted)
	return Latency{
		P50: percentile(sorted, 0.50),
		P95: percentile(sorted, 0.95),
		P99: percentile(sorted, 0.99),
		Max: sorted[len(sorted)-1],
	}
}

// percentile returns the nearest-rank percentile p of sorted.
func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(math.Ceil(p * float64(len(sorted))))
	return sorted[max(rank-1, 0)]
}
