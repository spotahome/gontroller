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
	ctrlDequeuedCnt    *prometheus.CounterVec
	ctrlQueuedCnt      *prometheus.CounterVec
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
		ctrlQueuedCnt: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsPrefix,
			Subsystem: "controller",
			Name:      "queue_queued_total",
			Help:      "The total number of queued objects on the queue.",
		}, []string{"controller"}),

		// Handy redundant metric (queued time histogram has also the counter).
		ctrlDequeuedCnt: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsPrefix,
			Subsystem: "controller",
			Name:      "queue_dequeued_total",
			Help:      "The total number of dequeued objects from the queue.",
		}, []string{"controller"}),

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
		p.ctrlQueuedCnt,
		p.ctrlDequeuedCnt,
		p.ctrlQueuedHist,
		p.ctrlListHist,
		p.ctrlStorageRetHist,
		p.ctrlHandledHist,
	)
}

func (p prometheusRecorder) WithID(id string) Recorder {
	return &prometheusRecorder{
		ctrlQueuedCnt:      p.ctrlQueuedCnt,
		ctrlDequeuedCnt:    p.ctrlDequeuedCnt,
		ctrlQueuedHist:     p.ctrlQueuedHist,
		ctrlStorageRetHist: p.ctrlStorageRetHist,
		ctrlListHist:       p.ctrlListHist,
		ctrlHandledHist:    p.ctrlHandledHist,
		controllerID:       id,
		reg:                p.reg,
	}
}

func (p prometheusRecorder) IncControllerQueuedTotal() {
	p.ctrlQueuedCnt.WithLabelValues(p.controllerID).Inc()
}

func (p prometheusRecorder) ObserveControllerOnQueueLatency(start time.Time) {
	p.ctrlDequeuedCnt.WithLabelValues(p.controllerID).Inc()
	p.ctrlQueuedHist.WithLabelValues(p.controllerID).Observe(time.Since(start).Seconds())
}

func (p prometheusRecorder) ObserveControllerStorageGetLatency(start time.Time, success bool) {
	p.ctrlStorageRetHist.WithLabelValues(p.controllerID, fmt.Sprint(success)).Observe(time.Since(start).Seconds())
}

func (p prometheusRecorder) ObserveControllerListLatency(start time.Time, success bool) {
	p.ctrlListHist.WithLabelValues(p.controllerID, fmt.Sprint(success)).Observe(time.Since(start).Seconds())
}

func (p prometheusRecorder) ObserveControllerHandleLatency(start time.Time, kind HandleKind, success bool) {
	p.ctrlHandledHist.WithLabelValues(p.controllerID, string(kind), fmt.Sprint(success)).Observe(time.Since(start).Seconds())
}
