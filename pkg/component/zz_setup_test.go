package component

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
)

func setupScheme() *runtime.Scheme {
	s := scheme.Scheme
	_ = AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

func setupReconcileContext(scheme *runtime.Scheme, owner *MockOperatorCRD, client client.Client) ReconcileContext {
	return ReconcileContext{
		Client:   client,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
		Metrics: &ocm.ConditionMetricRecorder{
			Controller:              "test-controller",
			OperatorConditionsGauge: ocm.NewOperatorConditionsGauge("test_namespace"),
		},
		Owner: owner,
	}
}
