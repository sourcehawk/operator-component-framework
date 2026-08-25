// Command simulator exposes a synthetic operator's metrics on /metrics for the
// local observability stack under observability/dev, and the kube-state-metrics
// series the dashboards join against on /ksm/metrics.
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
	"fmt"
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
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run wires the registry the way an operator would, serves /metrics and plays
// the world until the process is signalled. It returns an error when the
// metrics endpoint failed to serve, so main exits non-zero and
// `make observability-up` fails visibly instead of idling without metrics.
func run() error {
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
	mux.Handle("/ksm/metrics", promhttp.HandlerFor(newKubeStateMetrics(), promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: *listen, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("serving metrics on %s/metrics", *listen)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			stop()
		}
	}()

	newWorld(conditions, collectors, rt, *leader).run(ctx)

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	select {
	case err := <-serveErr:
		return fmt.Errorf("serving metrics: %w", err)
	default:
		return nil
	}
}
