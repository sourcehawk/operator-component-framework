package main

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Container request and limit values the simulated operator pod runs with,
// picked so that the Process panels show usage well inside the request and
// the limit at a visible distance above it.
const (
	simPodNamespace = "operators"
	simPodName      = "demo-operator-0"
	simContainer    = "manager"
	simCPURequest   = 0.25
	simCPULimit     = 1
	simMemRequest   = 128 << 20
	simMemLimit     = 512 << 20
)

// newKubeStateMetrics returns a registry holding lookalikes of the two
// kube-state-metrics families the OCF Operator dashboard joins against for
// container requests and limits, with the label set kube-state-metrics gives
// them. The pod and namespace match the target labels the dev Prometheus
// stamps on the simulator's own scrape, which is what the join keys on.
func newKubeStateMetrics() *prometheus.Registry {
	labels := []string{"namespace", "pod", "container", "node", "resource", "unit"}
	requests := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kube_pod_container_resource_requests",
		Help: "The number of requested request resource by a container.",
	}, labels)
	limits := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "kube_pod_container_resource_limits",
		Help: "The number of requested limit resource by a container.",
	}, labels)
	for _, r := range []struct {
		resource, unit string
		request, limit float64
	}{
		{"cpu", "core", simCPURequest, simCPULimit},
		{"memory", "byte", simMemRequest, simMemLimit},
	} {
		requests.WithLabelValues(simPodNamespace, simPodName, simContainer, "node-a", r.resource, r.unit).Set(r.request)
		limits.WithLabelValues(simPodNamespace, simPodName, simContainer, "node-a", r.resource, r.unit).Set(r.limit)
	}
	reg := prometheus.NewRegistry()
	reg.MustRegister(requests, limits)
	return reg
}
