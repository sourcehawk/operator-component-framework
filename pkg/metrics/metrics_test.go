package metrics_test

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/metrics"
)

func testLabels() component.ResourceMetricLabels {
	return component.ResourceMetricLabels{
		OwnerKind:  "WebApp",
		Component:  "web",
		Identifier: "tls",
		Kind:       "Secret",
	}
}

func TestRecorderSatisfiesMetricsRecorder(t *testing.T) {
	var recorder component.MetricsRecorder = metrics.NewRecorder("webapp-controller", nil, nil)
	assert.NotNil(t, recorder)
}

func TestRecordResourceApply(t *testing.T) {
	collectors := metrics.NewCollectors()
	recorder := metrics.NewRecorder("webapp-controller", nil, collectors)

	recorder.RecordResourceApply(testLabels(), concepts.ConvergingOperationNone)
	recorder.RecordResourceApply(testLabels(), concepts.ConvergingOperationNone)
	recorder.RecordResourceApply(testLabels(), concepts.ConvergingOperationUpdated)

	expected := `
# HELP ocf_resource_apply_total Framework applies of a managed resource, by outcome.
# TYPE ocf_resource_apply_total counter
ocf_resource_apply_total{component="web",controller="webapp-controller",kind="Secret",operation="none",owner_kind="WebApp",resource="tls"} 2
ocf_resource_apply_total{component="web",controller="webapp-controller",kind="Secret",operation="updated",owner_kind="WebApp",resource="tls"} 1
`
	require.NoError(t, testutil.CollectAndCompare(
		collectors, strings.NewReader(expected), "ocf_resource_apply_total",
	))
}

func TestRecordResourceApplyLowercasesTheOperation(t *testing.T) {
	collectors := metrics.NewCollectors()
	recorder := metrics.NewRecorder("webapp-controller", nil, collectors)

	recorder.RecordResourceApply(testLabels(), concepts.ConvergingOperationCreated)

	expected := `
# HELP ocf_resource_apply_total Framework applies of a managed resource, by outcome.
# TYPE ocf_resource_apply_total counter
ocf_resource_apply_total{component="web",controller="webapp-controller",kind="Secret",operation="created",owner_kind="WebApp",resource="tls"} 1
`
	require.NoError(t, testutil.CollectAndCompare(
		collectors, strings.NewReader(expected), "ocf_resource_apply_total",
	))
}

func TestRecordResourceApplyError(t *testing.T) {
	collectors := metrics.NewCollectors()
	recorder := metrics.NewRecorder("webapp-controller", nil, collectors)

	recorder.RecordResourceApplyError(testLabels())

	expected := `
# HELP ocf_resource_apply_errors_total Failed framework applies of a managed resource.
# TYPE ocf_resource_apply_errors_total counter
ocf_resource_apply_errors_total{component="web",controller="webapp-controller",kind="Secret",owner_kind="WebApp",resource="tls"} 1
`
	require.NoError(t, testutil.CollectAndCompare(
		collectors, strings.NewReader(expected), "ocf_resource_apply_errors_total",
	))
}

func TestSeparateControllersShareCollectors(t *testing.T) {
	collectors := metrics.NewCollectors()
	web := metrics.NewRecorder("webapp-controller", nil, collectors)
	db := metrics.NewRecorder("db-controller", nil, collectors)

	web.RecordResourceApply(testLabels(), concepts.ConvergingOperationNone)
	db.RecordResourceApply(testLabels(), concepts.ConvergingOperationNone)

	expected := `
# HELP ocf_resource_apply_total Framework applies of a managed resource, by outcome.
# TYPE ocf_resource_apply_total counter
ocf_resource_apply_total{component="web",controller="db-controller",kind="Secret",operation="none",owner_kind="WebApp",resource="tls"} 1
ocf_resource_apply_total{component="web",controller="webapp-controller",kind="Secret",operation="none",owner_kind="WebApp",resource="tls"} 1
`
	require.NoError(t, testutil.CollectAndCompare(
		collectors, strings.NewReader(expected), "ocf_resource_apply_total",
	))
}

func TestNilCollectorsSkipResourceMetrics(t *testing.T) {
	recorder := metrics.NewRecorder("webapp-controller", nil, nil)

	assert.NotPanics(t, func() {
		recorder.RecordResourceApply(testLabels(), concepts.ConvergingOperationCreated)
		recorder.RecordResourceApplyError(testLabels())
	})
}

func TestNilConditionsGaugeSkipsConditionMetrics(t *testing.T) {
	recorder := metrics.NewRecorder("webapp-controller", nil, metrics.NewCollectors())

	assert.NotPanics(t, func() {
		recorder.RecordConditionFor("WebApp", fakeObject{}, "Ready", "True", "AllGood", time.Now())
	})
	assert.Zero(t, recorder.RemoveConditionsFor("WebApp", fakeObject{}))
}

func TestConditionMetricsAreRecordedThroughTheGauge(t *testing.T) {
	gauge := ocm.NewOperatorConditionsGauge("ocf_test")
	recorder := metrics.NewRecorder("webapp-controller", gauge, nil)

	recorder.RecordConditionFor("WebApp", fakeObject{}, "Ready", "True", "AllGood", time.Now())

	assert.Equal(t, 1, testutil.CollectAndCount(gauge))
	assert.Equal(t, 1, recorder.RemoveConditionsFor("WebApp", fakeObject{}))
	assert.Zero(t, testutil.CollectAndCount(gauge))
}

// fakeObject is the minimal ocm.ObjectLike the condition recorder needs.
type fakeObject struct{}

func (fakeObject) GetName() string      { return "app" }
func (fakeObject) GetNamespace() string { return "default" }

var _ ocm.ObjectLike = fakeObject{}
