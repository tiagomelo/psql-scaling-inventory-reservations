// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

// Package metrics defines the Prometheus metrics exposed by the benchmark.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the benchmark's Prometheus collectors.
type Metrics struct {
	ReservationDuration *prometheus.HistogramVec
	Reservations        *prometheus.CounterVec
	Workers             *prometheus.GaugeVec
}

// New creates the collectors and registers them in reg.
func New(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		ReservationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "inventory_reservation_duration_seconds",
				Help: "Time required to reserve inventory",
				Buckets: prometheus.ExponentialBuckets(
					0.0005,
					2,
					18,
				),
			},
			[]string{"strategy"},
		),
		Reservations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "inventory_reservations_total",
				Help: "Inventory reservation attempts",
			},
			[]string{"strategy", "result"},
		),
		Workers: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "inventory_bench_workers",
				Help: "Number of concurrent workers in the current benchmark run",
			},
			[]string{"strategy"},
		),
	}
	reg.MustRegister(m.ReservationDuration, m.Reservations, m.Workers)
	return m
}
