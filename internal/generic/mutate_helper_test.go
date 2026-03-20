package generic

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type orderRecorder struct {
	events []string
}

func (r *orderRecorder) record(event string) {
	r.events = append(r.events, event)
}

type recordingMutator struct {
	recorder *orderRecorder
}

func (m *recordingMutator) Apply() error {
	m.recorder.record("mutator.Apply")
	return nil
}

func (m *recordingMutator) beginFeature() {
	m.recorder.record("mutator.beginFeature")
}

func TestApplyMutationsOrder(t *testing.T) {
	recorder := &orderRecorder{}

	current := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}
	desired := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
	}

	defaultApplicator := func(_, _ *corev1.ConfigMap) error {
		recorder.record("defaultApplicator")
		return nil
	}

	flavors := []FieldApplicationFlavor[*corev1.ConfigMap]{
		func(_, _, _ *corev1.ConfigMap) error {
			recorder.record("flavor1")
			return nil
		},
	}

	newMutator := func(_ *corev1.ConfigMap) *recordingMutator {
		recorder.record("newMutator")
		return &recordingMutator{recorder: recorder}
	}

	mutations := []feature.Mutation[*recordingMutator]{
		{
			Name:    "feat1",
			Feature: feature.NewResourceFeature("1.0.0", nil),
			Mutate: func(_ *recordingMutator) error {
				recorder.record("mutation1")
				return nil
			},
		},
	}

	suspender := func(_ *recordingMutator) error {
		recorder.record("suspender")
		return nil
	}

	_, err := ApplyMutations[*corev1.ConfigMap, *recordingMutator](
		current,
		desired,
		defaultApplicator,
		nil,
		flavors,
		newMutator,
		mutations,
		suspender,
	)

	if err != nil {
		t.Fatalf("ApplyMutations failed: %v", err)
	}

	expectedOrder := []string{
		"defaultApplicator",
		"flavor1",
		"newMutator",
		"mutator.beginFeature",
		"mutation1",
		"mutator.Apply",
		"mutator.beginFeature",
		"suspender",
		"mutator.Apply",
	}

	if len(recorder.events) != len(expectedOrder) {
		t.Fatalf("expected %d events, got %d: %v", len(expectedOrder), len(recorder.events), recorder.events)
	}

	for i, event := range expectedOrder {
		if recorder.events[i] != event {
			t.Errorf("at index %d: expected %s, got %s", i, event, recorder.events[i])
		}
	}
}
