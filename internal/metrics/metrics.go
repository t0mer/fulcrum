// Package metrics defines Fulcrum's Prometheus collectors.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics bundles the application collectors. Register once at startup and
// pass the struct to the components that emit values.
type Metrics struct {
	InboundMessages *prometheus.CounterVec
	ImagesProcessed prometheus.Counter
	FacesDetected   prometheus.Counter
	Matches         *prometheus.CounterVec
	EmbedLatency    prometheus.Histogram
	QueueDepth      prometheus.Gauge
	JobFailures     prometheus.Counter
	SinkErrors      *prometheus.CounterVec
}

// New registers and returns the collectors on the given registerer.
func New(reg prometheus.Registerer) *Metrics {
	f := promauto.With(reg)
	return &Metrics{
		InboundMessages: f.NewCounterVec(prometheus.CounterOpts{
			Name: "fulcrum_inbound_messages_total",
			Help: "Inbound webhook messages received.",
		}, []string{"provider", "group"}),
		ImagesProcessed: f.NewCounter(prometheus.CounterOpts{
			Name: "fulcrum_images_processed_total",
			Help: "Images pulled from the queue and processed.",
		}),
		FacesDetected: f.NewCounter(prometheus.CounterOpts{
			Name: "fulcrum_faces_detected_total",
			Help: "Faces detected by the ML sidecar.",
		}),
		Matches: f.NewCounterVec(prometheus.CounterOpts{
			Name: "fulcrum_matches_total",
			Help: "Matches produced, by subject.",
		}, []string{"subject"}),
		EmbedLatency: f.NewHistogram(prometheus.HistogramOpts{
			Name:    "fulcrum_embed_latency_seconds",
			Help:    "Latency of a /detect call to the ML sidecar.",
			Buckets: prometheus.DefBuckets,
		}),
		QueueDepth: f.NewGauge(prometheus.GaugeOpts{
			Name: "fulcrum_queue_depth",
			Help: "Pending jobs in the queue.",
		}),
		JobFailures: f.NewCounter(prometheus.CounterOpts{
			Name: "fulcrum_job_failures_total",
			Help: "Jobs that exhausted their retries.",
		}),
		SinkErrors: f.NewCounterVec(prometheus.CounterOpts{
			Name: "fulcrum_sink_errors_total",
			Help: "Sink delivery errors, by sink.",
		}, []string{"sink"}),
	}
}
