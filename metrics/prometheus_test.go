package metrics_test

import (
	"io/ioutil"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stretchr/testify/assert"

	"github.com/spotahome/gontroller/metrics"
)

func TestPrometheusRecorder(t *testing.T) {
	tests := []struct {
		name       string
		addMetrics func(metricssvc metrics.Recorder)
		expMetrics []string
	}{
		{
			name: "Measuring controller queue metrics should generate the correct metrics.",
			addMetrics: func(svc metrics.Recorder) {
				now := time.Now()
				svc.ObserveControllerOnQueueLatency(now.Add(-1 * time.Second))
				svc.ObserveControllerOnQueueLatency(now.Add(-6 * time.Second))
				svc.ObserveControllerOnQueueLatency(now.Add(-55 * time.Millisecond))

			},
			expMetrics: []string{
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="0.005"} 0`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="0.01"} 0`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="0.025"} 0`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="0.05"} 0`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="0.1"} 1`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="0.25"} 1`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="0.5"} 1`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="1"} 1`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="2.5"} 2`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="5"} 2`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="10"} 3`,
				`gontroller_controller_queued_duration_seconds_bucket{controller="test",le="+Inf"} 3`,
				`gontroller_controller_queued_duration_seconds_count{controller="test"} 3`,

				`gontroller_controller_queue_dequeued_total{controller="test"} 3`,
			},
		},
		{
			name: "Measuring controller handle metrics should generate the correct metrics.",
			addMetrics: func(svc metrics.Recorder) {
				svc.IncControllerQueuedTotal()
				svc.IncControllerQueuedTotal()
				svc.IncControllerQueuedTotal()
			},
			expMetrics: []string{
				`gontroller_controller_queue_queued_total{controller="test"} 3`,
			},
		},
		{
			name: "Measuring controller listerwatcher list metrics should generate the correct metrics.",
			addMetrics: func(svc metrics.Recorder) {
				now := time.Now()
				svc.ObserveControllerListLatency(now.Add(-1*time.Second), true)
				svc.ObserveControllerListLatency(now.Add(-6*time.Second), false)
				svc.ObserveControllerListLatency(now.Add(-55*time.Millisecond), true)

			},
			expMetrics: []string{
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="0.005"}`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="0.01"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="0.025"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="0.05"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="0.1"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="0.25"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="0.5"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="1"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="2.5"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="5"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="10"} 1`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="false",le="+Inf"} 1`,
				`gontroller_controller_listerwatcher_list_duration_seconds_count{controller="test",success="false"} 1`,

				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="0.005"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="0.01"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="0.025"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="0.05"} 0`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="0.1"} 1`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="0.25"} 1`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="0.5"} 1`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="1"} 1`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="2.5"} 2`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="5"} 2`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="10"} 2`,
				`gontroller_controller_listerwatcher_list_duration_seconds_bucket{controller="test",success="true",le="+Inf"} 2`,
				`gontroller_controller_listerwatcher_list_duration_seconds_count{controller="test",success="true"} 2`,
			},
		},
		{
			name: "Measuring controller storage get metrics should generate the correct metrics.",
			addMetrics: func(svc metrics.Recorder) {
				now := time.Now()
				svc.ObserveControllerStorageGetLatency(now.Add(-1*time.Second), true)
				svc.ObserveControllerStorageGetLatency(now.Add(-6*time.Second), false)
				svc.ObserveControllerStorageGetLatency(now.Add(-55*time.Millisecond), true)

			},
			expMetrics: []string{
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="0.005"}`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="0.01"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="0.025"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="0.05"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="0.1"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="0.25"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="0.5"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="1"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="2.5"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="5"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="10"} 1`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="false",le="+Inf"} 1`,
				`gontroller_controller_storage_get_duration_seconds_count{controller="test",success="false"} 1`,

				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="0.005"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="0.01"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="0.025"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="0.05"} 0`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="0.1"} 1`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="0.25"} 1`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="0.5"} 1`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="1"} 1`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="2.5"} 2`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="5"} 2`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="10"} 2`,
				`gontroller_controller_storage_get_duration_seconds_bucket{controller="test",success="true",le="+Inf"} 2`,
				`gontroller_controller_storage_get_duration_seconds_count{controller="test",success="true"} 2`,
			},
		},
		{
			name: "Measuring controller handle metrics should generate the correct metrics.",
			addMetrics: func(svc metrics.Recorder) {
				now := time.Now()
				svc.ObserveControllerHandleLatency(now.Add(-1*time.Second), metrics.AddHandleKind, true)
				svc.ObserveControllerHandleLatency(now.Add(-55*time.Millisecond), metrics.AddHandleKind, false)
				svc.ObserveControllerHandleLatency(now.Add(-6*time.Second), metrics.DeleteHandleKind, true)
				svc.ObserveControllerHandleLatency(now.Add(-1*time.Millisecond), metrics.DeleteHandleKind, true)
				svc.ObserveControllerHandleLatency(now.Add(-9*time.Second), metrics.DeleteHandleKind, true)

			},
			expMetrics: []string{
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="0.005"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="0.01"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="0.025"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="0.05"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="0.1"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="0.25"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="0.5"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="1"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="2.5"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="5"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="10"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="false",le="+Inf"} 1`,
				`gontroller_controller_handle_duration_seconds_count{controller="test",kind="add",success="false"} 1`,

				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="0.005"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="0.01"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="0.025"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="0.05"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="0.1"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="0.25"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="0.5"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="1"} 0`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="2.5"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="5"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="10"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="add",success="true",le="+Inf"} 1`,
				`gontroller_controller_handle_duration_seconds_count{controller="test",kind="add",success="true"} 1`,

				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="0.005"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="0.01"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="0.025"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="0.05"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="0.1"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="0.25"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="0.5"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="1"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="2.5"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="5"} 1`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="10"} 3`,
				`gontroller_controller_handle_duration_seconds_bucket{controller="test",kind="delete",success="true",le="+Inf"} 3`,
				`gontroller_controller_handle_duration_seconds_count{controller="test",kind="delete",success="true"} 3`,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			// Create the prometheus registry and metrics Recorder.
			reg := prometheus.NewRegistry()
			svc := metrics.NewPrometheus(reg).WithID("test")

			// Measure.
			test.addMetrics(svc)

			// Get the metrics from Prometheus.
			h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/metrics", nil)
			h.ServeHTTP(w, r)

			// Check the obtained metrics
			metrics, _ := ioutil.ReadAll(w.Result().Body)
			for _, expMetric := range test.expMetrics {
				assert.Contains(string(metrics), expMetric)
			}
		})
	}
}
