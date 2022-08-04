package ocrworker

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	inFlightGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "ocr_in_flight_requests",
		Help: "Number of currently pending and processed requests.",
	})
	counter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ocr_api_requests_total",
			Help: "A counter for requests to the wrapped handler.",
		},
		[]string{"code", "method"},
	)

	// duration is partitioned by the HTTP method and handler. It uses custom
	// buckets based on the expected request duration.
	duration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ocr_request_duration_seconds",
			Help:    "A histogram of latencies for requests.",
			Buckets: []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"handler", "method"},
	)

	// requestSize has no labels, making it a zero-dimensional ObserverVec.
	requestSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ocr_request_size_bytes",
			Help:    "A histogram of response sizes for requests.",
			Buckets: []float64{100, 1500, 5000000, 10000000, 25000000, 50000000},
		},
		[]string{},
	)
)

// InstrumentHttpStatusHandler wraps httpHandler to provide prometheus metrics
func InstrumentHttpStatusHandler(ocrHttpHandler *OcrHTTPStatusHandler) http.Handler {
	// Register all the metrics in the standard registry.
	prometheus.MustRegister(inFlightGauge, counter, duration, requestSize)

	ocrChain := promhttp.InstrumentHandlerInFlight(inFlightGauge,
		promhttp.InstrumentHandlerDuration(duration.MustCurryWith(prometheus.Labels{"handler": "ocr"}),
			promhttp.InstrumentHandlerCounter(counter,
				// promhttp.InstrumentHandlerRequestSize(requestSize, ocrworker.NewOcrHttpHandler(rabbitConfig)),
				promhttp.InstrumentHandlerRequestSize(requestSize, ocrHttpHandler),
			),
		),
	)
	return ocrChain
}
