package main

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	clientmetrics "k8s.io/client-go/tools/metrics"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// family is the shape of a metric family that the dashboards and alerts
// depend on: its type, its label names and, for histograms, its buckets.
type family struct {
	kind    dto.MetricType
	labels  []string
	buckets []float64
}

func families(t *testing.T, g prometheus.Gatherer, prefixes ...string) map[string]family {
	t.Helper()
	mfs, err := g.Gather()
	require.NoError(t, err)
	out := map[string]family{}
	for _, mf := range mfs {
		name := mf.GetName()
		matched := false
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				matched = true
			}
		}
		if !matched || len(mf.GetMetric()) == 0 {
			continue
		}
		m := mf.GetMetric()[0]
		f := family{kind: mf.GetType()}
		for _, lp := range m.GetLabel() {
			f.labels = append(f.labels, lp.GetName())
		}
		sort.Strings(f.labels)
		if h := m.GetHistogram(); h != nil {
			for _, b := range h.GetBucket() {
				f.buckets = append(f.buckets, b.GetUpperBound())
			}
		}
		out[name] = f
	}
	return out
}

// TestRuntimeMetricsMatchControllerRuntime starts a real, unmanaged
// controller-runtime controller (no API server involved) and drives one
// request through its queue so that controller-runtime initialises its
// reconcile and workqueue series, then checks that every lookalike series the
// simulator defines exists in controller-runtime with the same type, label
// names and buckets.
func TestRuntimeMetricsMatchControllerRuntime(t *testing.T) {
	reconciled := make(chan struct{})
	var once sync.Once
	c, err := controller.NewUnmanaged("parity", controller.Options{
		Reconciler: reconcile.Func(func(context.Context, reconcile.Request) (reconcile.Result, error) {
			once.Do(func() { close(reconciled) })
			return reconcile.Result{}, nil
		}),
		SkipNameValidation: ptr.To(true),
	})
	require.NoError(t, err)
	require.NoError(t, c.Watch(source.Func(func(_ context.Context, q workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
		q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "parity"}})
		return nil
	})))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx) }()
	select {
	case <-reconciled:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "the controller never reconciled the enqueued request")
	}
	// ReconcileTime is observed after the reconciler returns; wait for the
	// family to appear before snapshotting the registry.
	require.Eventually(t, func() bool {
		mfs, err := ctrlmetrics.Registry.Gather()
		if err != nil {
			return false
		}
		for _, mf := range mfs {
			if mf.GetName() == "controller_runtime_reconcile_time_seconds" && len(mf.GetMetric()) > 0 {
				return true
			}
		}
		return false
	}, 10*time.Second, 10*time.Millisecond, "controller-runtime never observed a reconcile duration")
	cancel()
	require.NoError(t, <-done)

	// client-go creates rest_client_requests_total lazily on the first request;
	// controller-runtime's pkg/metrics init installed the adapter, so one
	// recorded result materialises the real family in the registry.
	clientmetrics.RequestResult.Increment(context.Background(), "200", "GET", "10.96.0.1:443")

	want := families(t, ctrlmetrics.Registry, "controller_runtime_", "workqueue_", "rest_client_", "leader_election_")

	reg := prometheus.NewRegistry()
	rm := newRuntimeMetrics(reg)
	rm.initController("parity", 1)
	rm.restRequests.WithLabelValues("200", "GET", "https://example").Add(0)
	rm.leader.WithLabelValues("parity").Set(1)
	got := families(t, reg, "controller_runtime_", "workqueue_", "rest_client_", "leader_election_")

	require.NotEmpty(t, got)
	for name, g := range got {
		w, ok := want[name]
		if !ok && strings.HasPrefix(name, "leader_election_") {
			// The leader elector creates leader_election_master_status only
			// when it runs against an API server. Its definition is kept in
			// step with controller-runtime's leaderelection.go by hand.
			continue
		}
		if !assert.True(t, ok, "simulator metric %s does not exist in controller-runtime", name) {
			continue
		}
		assert.Equal(t, w.kind, g.kind, "%s type", name)
		assert.Equal(t, w.labels, g.labels, "%s labels", name)
		assert.Equal(t, w.buckets, g.buckets, "%s buckets", name)
	}
}
