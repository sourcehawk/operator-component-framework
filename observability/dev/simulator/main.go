// Command simulator exposes a synthetic operator's metrics on /metrics for the
// local observability stack under observability/dev.
//
// The framework's own series (resource applies, conditions) are recorded
// through the real pkg/metrics recorder; controller-runtime, workqueue, REST
// client and leader election series are lookalikes guarded by runtime_test.go.
// The world it plays is described in docs/observability.md.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"

	"github.com/sourcehawk/operator-component-framework/pkg/metrics"
)

func main() {
	listen := flag.String("listen", ":8080", "address to serve /metrics on")
	metricNamespace := flag.String("metric-namespace", "demo", "metric namespace of the condition gauge")
	leader := flag.Bool("leader", true, "report this replica as the leader; false shows OperatorLeaderMissing")
	flag.Parse()

	reg := prometheus.NewRegistry()
	conditions := ocm.NewOperatorConditionsGauge(*metricNamespace)
	collectors := metrics.NewCollectors()
	reg.MustRegister(conditions, collectors)
	rt := newRuntimeMetrics(reg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("serving metrics on %s/metrics", *listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("serve: %v", err)
			stop()
		}
	}()

	newWorld(conditions, collectors, rt, *leader).run(ctx)

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
