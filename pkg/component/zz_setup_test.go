package component

import (
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ocm "github.com/sourcehawk/go-crd-condition-metrics/pkg/crd-condition-metrics"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// recordedEvent captures every argument handed to [events.EventRecorder.Eventf].
// The client-go fake recorder collapses events into a string and drops the
// related object and the action, both of which are asserted on here.
type recordedEvent struct {
	regarding runtime.Object
	related   runtime.Object
	eventType string
	reason    string
	action    string
	note      string
}

// spyRecorder is an [events.EventRecorder] that records events in memory.
type spyRecorder struct {
	mu     sync.Mutex
	events []recordedEvent
}

var _ events.EventRecorder = &spyRecorder{}

func (s *spyRecorder) Eventf(
	regarding, related runtime.Object, eventtype, reason, action, note string, args ...any,
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, recordedEvent{
		regarding: regarding,
		related:   related,
		eventType: eventtype,
		reason:    reason,
		action:    action,
		note:      fmt.Sprintf(note, args...),
	})
}

// recorded returns a copy of the events captured so far.
func (s *spyRecorder) recorded() []recordedEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedEvent(nil), s.events...)
}

// recordedWithReason returns the captured events matching the given reason.
func (s *spyRecorder) recordedWithReason(reason string) []recordedEvent {
	var matching []recordedEvent
	for _, event := range s.recorded() {
		if event.reason == reason {
			matching = append(matching, event)
		}
	}
	return matching
}

func setupScheme() *runtime.Scheme {
	s := scheme.Scheme
	_ = AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

func setupReconcileContext(scheme *runtime.Scheme, owner *MockOperatorCRD, client client.Client) ReconcileContext {
	return ReconcileContext{
		Client:        client,
		Scheme:        scheme,
		EventRecorder: &spyRecorder{},
		Metrics:       &spyMetrics{},
		Owner:         owner,
	}
}

// recordedApply captures one call to [MetricsRecorder.RecordResourceApply].
type recordedApply struct {
	labels    ResourceMetricLabels
	operation concepts.ConvergingOperation
}

// spyMetrics is a [MetricsRecorder] that captures resource-level emissions in
// memory. Condition recording is exercised separately through MockMetrics.
type spyMetrics struct {
	mu      sync.Mutex
	applies []recordedApply
	errors  []ResourceMetricLabels
}

var _ MetricsRecorder = &spyMetrics{}

func (s *spyMetrics) RecordConditionFor(
	string, ocm.ObjectLike, string, string, string, time.Time, ...string,
) {
}

func (s *spyMetrics) RecordResourceApply(labels ResourceMetricLabels, operation concepts.ConvergingOperation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.applies = append(s.applies, recordedApply{labels: labels, operation: operation})
}

func (s *spyMetrics) RecordResourceApplyError(labels ResourceMetricLabels) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.errors = append(s.errors, labels)
}

// recordedApplies returns a copy of the applies captured so far.
func (s *spyMetrics) recordedApplies() []recordedApply {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedApply(nil), s.applies...)
}

// recordedErrors returns a copy of the apply errors captured so far.
func (s *spyMetrics) recordedErrors() []ResourceMetricLabels {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ResourceMetricLabels(nil), s.errors...)
}
