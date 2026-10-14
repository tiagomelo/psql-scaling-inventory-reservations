// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package bench

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSummarize(t *testing.T) {
	testCases := []struct {
		name      string
		durations []time.Duration
		expected  Latency
	}{
		{
			name:      "empty",
			durations: nil,
			expected:  Latency{},
		},
		{
			name:      "single value",
			durations: []time.Duration{time.Millisecond},
			expected: Latency{
				P50: time.Millisecond,
				P95: time.Millisecond,
				P99: time.Millisecond,
				Max: time.Millisecond,
			},
		},
		{
			name: "one hundred values in reverse order",
			durations: func() []time.Duration {
				d := make([]time.Duration, 100)
				for i := range d {
					d[i] = time.Duration(100-i) * time.Millisecond
				}
				return d
			}(),
			expected: Latency{
				P50: 50 * time.Millisecond,
				P95: 95 * time.Millisecond,
				P99: 99 * time.Millisecond,
				Max: 100 * time.Millisecond,
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, Summarize(tc.durations))
		})
	}
}
