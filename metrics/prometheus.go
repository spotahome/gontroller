package metrics

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricsPrefix = "gontroller"
)

type prometheusRecorder struct {
	ctrlQueuedHist     *prometheus.HistogramVec
	ctrlStorageRetHist *prometheus.HistogramVec
	ctrlListHist       *prometheus.HistogramVec
	ctrlHandledHist    *prometheus.HistogramVec

	controllerID string
	reg          prometheus.Registerer
}

// NewPrometheus returns a metrics.Recorder that implements it using Prometheus
// as the metrics backend.
func NewPrometheus(reg prometheus.Registerer) Recorder {
	p := &prometheusRecorder{
		ctrlQueuedHist: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsPrefix,
			Subsystem: "controller",
			Name:      "queued_duration_seconds",
			Help:      "The latency in seconds of the object queued in the controller before being handled.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"controller"}),

		ctrlStorageRetHist: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsPrefix,
			Subsystem: "controller",
			Name:      "storage_get_duration_seconds",
			Help:      "The latency in seconds of the controller using the storage to get object data to be handled.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"controller", "success"}),

		ctrlListHist: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsPrefix,
			Subsystem: "controller",
			Name:      "listerwatcher_list_duration_seconds",
			Help:      "The latency in seconds of the controller using the listerwatcher to list objects before being added to the controller queue.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"controller", "success"}),

		ctrlHandledHist: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsPrefix,
			Subsystem: "controller",
			Name:      "handle_duration_seconds",
			Help:      "The latency in seconds of the controller using the handler to handle objects.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"controller", "kind", "success"}),

		reg: reg,
	}

	p.registerMetrics()

	return p
}

func (p prometheusRecorder) registerMetrics() {
	p.reg.MustRegister(
		p.ctrlQueuedHist,
		p.ctrlListHist,
		p.ctrlStorageRetHist,
		p.ctrlHandledHist,
	)
}

func (p prometheusRecorder) WithID(id string) Recorder {
	pcopy := NewPrometheus(p.reg).(*prometheusRecorder)
	pcopy.controllerID = id
	return pcopy
}

func (p prometheusRecorder) ObserveControllerOnQueueLatency(start time.Time) {
	p.ctrlQueuedHist.WithLabelValues(p.controllerID).Observe(time.Since(start).Seconds())
}

func (p prometheusRecorder) ObserveControllerStorageGetLatency(start time.Time, success bool) {
	p.ctrlStorageRetHist.WithLabelValues(p.controllerID, fmt.Sprint(success)).Observe(time.Since(start).Seconds())
}

func (p prometheusRecorder) ObserveControllerListLatency(start time.Time, success bool) {
	p.ctrlListHist.WithLabelValues(p.controllerID, fmt.Sprint(success)).Observe(time.Since(start).Seconds())
}

func (p prometheusRecorder) ObserveControllerHandleLatency(start time.Time, kind string, success bool) {
	p.ctrlHandledHist.WithLabelValues(p.controllerID, kind, fmt.Sprint(success)).Observe(time.Since(start).Seconds())
}
