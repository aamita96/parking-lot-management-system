// Package metrics declares the Prometheus metrics listed in NFR-12.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "plms_http_request_duration_seconds",
		Help:    "HTTP request latency by route.",
		Buckets: []float64{.005, .01, .025, .05, .1, .15, .3, .5, 1, 2.5},
	}, []string{"method", "route", "status"})

	Scans = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "plms_gate_scans_total",
		Help: "Gate scans by kind, result and reject reason.",
	}, []string{"kind", "result", "reason"})

	Payments = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "plms_payments_total",
		Help: "Closed tickets by payment method.",
	}, []string{"method"})

	PaymentsPaise = promauto.NewCounter(prometheus.CounterOpts{
		Name: "plms_payments_paise_total",
		Help: "Money collected (paise), excluding simulated tickets.",
	})

	Occupancy = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "plms_occupancy_ratio",
		Help: "Occupied / in-service spots by spot type.",
	}, []string{"spot_type"})

	OutboxLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "plms_outbox_lag_seconds",
		Help: "Age of the oldest unpublished outbox event (0 when empty).",
	})

	OutboxPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "plms_outbox_published_total",
		Help: "Outbox publish attempts by outcome.",
	}, []string{"outcome"})

	Emails = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "plms_emails_total",
		Help: "Email sends by kind and resulting status.",
	}, []string{"kind", "status"})

	ConsumerLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "plms_consumer_lag_messages",
		Help: "Notification consumer group lag summed over partitions.",
	})
)
