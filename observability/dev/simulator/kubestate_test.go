package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The dashboards join kube_pod_container_resource_* to the operator's scrape
// on (namespace, pod), so the lookalikes must carry kube-state-metrics' label
// set and name the pod the dev Prometheus stamps on the simulator's target.
func TestKubeStateMetricsLookalikes(t *testing.T) {
	mfs, err := newKubeStateMetrics().Gather()
	require.NoError(t, err)
	got := map[string]int{}
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			assert.Equal(t, simPodNamespace, labels["namespace"], "%s namespace", mf.GetName())
			assert.Equal(t, simPodName, labels["pod"], "%s pod", mf.GetName())
			assert.Contains(t, []string{"cpu", "memory"}, labels["resource"], "%s resource", mf.GetName())
			assert.NotEmpty(t, labels["container"])
			assert.NotEmpty(t, labels["unit"])
			got[mf.GetName()]++
		}
	}
	assert.Equal(t, map[string]int{
		"kube_pod_container_resource_requests": 2,
		"kube_pod_container_resource_limits":   2,
	}, got)
}
