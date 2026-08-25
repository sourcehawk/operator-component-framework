package main

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/stretchr/testify/require"

	"github.com/sourcehawk/operator-component-framework/pkg/metrics"
)

// seriesValue finds the first series of the named family whose labels are a
// superset of want and returns its gauge or counter value.
func seriesValue(mfs []*dto.MetricFamily, name string, want map[string]string) (float64, bool) {
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			matched := true
			for k, v := range want {
				if labels[k] != v {
					matched = false
					break
				}
			}
			if !matched {
				continue
			}
			switch {
			case m.GetGauge() != nil:
				return m.GetGauge().GetValue(), true
			case m.GetCounter() != nil:
				return m.GetCounter().GetValue(), true
			}
			return 0, true
		}
	}
	return 0, false
}

// TestWorldScriptedScenarios wires a registry the way main does, runs the
// world and checks that every scenario's series appear with the shapes the
// dev stack's alerts key on: the stuck Ready condition with its eight hour
// old transition, the failing PVC applies, the hot loop's rising updated
// counter, the cluster-scoped owner without a namespace label, both
// controllers reconciling and the leader gauge at one. It then cancels the
// context and requires run to return.
func TestWorldScriptedScenarios(t *testing.T) {
	reg := prometheus.NewRegistry()
	conditions := ocm.NewOperatorConditionsGauge("demo")
	collectors := metrics.NewCollectors()
	reg.MustRegister(conditions, collectors)
	rt := newRuntimeMetrics(reg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	w := newWorld(conditions, collectors, rt, true)
	// The backlog and panic scenarios run on minute-scale timers in the dev
	// stack; shorten them so the test sees both fire more than once.
	w.backlogInterval = 50 * time.Millisecond
	w.panicDelay = 50 * time.Millisecond
	w.panicInterval = 50 * time.Millisecond
	go func() {
		w.run(ctx)
		close(done)
	}()

	var firstUpdated float64
	require.Eventually(t, func() bool {
		mfs, err := reg.Gather()
		if err != nil {
			return false
		}
		stuck, ok := seriesValue(mfs, "demo_controller_condition", map[string]string{
			"kind": "Database", "name": "orders-db", "condition": "Ready", "status": "False", "reason": "Failing",
		})
		if !ok || time.Since(time.Unix(int64(stuck), 0)) < 7*time.Hour {
			return false
		}
		if _, ok := seriesValue(mfs, "ocf_resource_apply_errors_total", map[string]string{"resource": "pvc"}); !ok {
			return false
		}
		updated, ok := seriesValue(mfs, "ocf_resource_apply_total", map[string]string{
			"resource": "configmap", "operation": "updated",
		})
		if !ok || updated == 0 {
			return false
		}
		if firstUpdated == 0 {
			firstUpdated = updated
			return false // seen once; wait until it has risen
		}
		if updated <= firstUpdated {
			return false
		}
		// The cluster-scoped owner: client_golang exports the declared
		// namespace label with an empty value, which Prometheus ingests as no
		// namespace label at all, and the id keeps its "<ns>/<name>" shape.
		if _, ok := seriesValue(mfs, "demo_controller_condition", map[string]string{
			"kind": "Database", "name": "shared-gateway", "namespace": "",
			"id": "/shared-gateway", "condition": "Ready", "status": "True",
		}); !ok {
			return false
		}
		for _, controller := range []string{"webapp", "database"} {
			if _, ok := seriesValue(mfs, "controller_runtime_reconcile_total", map[string]string{"controller": controller}); !ok {
				return false
			}
		}
		// The backlog scenario keeps the database queue deep with items waiting
		// and both workers busy, and the panic scenario keeps counting.
		depth, ok := seriesValue(mfs, "workqueue_depth", map[string]string{"controller": "database"})
		if !ok || depth < 30 {
			return false
		}
		if _, ok := seriesValue(mfs, "workqueue_queue_duration_seconds", map[string]string{"controller": "database"}); !ok {
			return false
		}
		if active, ok := seriesValue(mfs, "controller_runtime_active_workers", map[string]string{"controller": "database"}); !ok || active != 2 {
			return false
		}
		if panics, ok := seriesValue(mfs, "controller_runtime_reconcile_panics_total", map[string]string{"controller": "database"}); !ok || panics < 2 {
			return false
		}
		leader, ok := seriesValue(mfs, "leader_election_master_status", map[string]string{"name": "demo-operator"})
		return ok && leader == 1
	}, 5*time.Second, 100*time.Millisecond, "the scripted world never produced the expected series")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "run did not return after the context was cancelled")
	}
}
