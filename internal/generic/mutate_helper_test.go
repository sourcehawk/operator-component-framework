package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (m *recordingMutator) BeginFeature() {
	m.recorder.record("mutator.BeginFeature")
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

	mutations := []Mutation[*recordingMutator]{
		{
			Name:    "feat1",
			Feature: alwaysEnabled{},
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

	require.NoError(t, err)

	expectedOrder := []string{
		"defaultApplicator",
		"flavor1",
		"newMutator",
		"mutator.BeginFeature",
		"mutation1",
		"mutator.Apply",
		"mutator.BeginFeature",
		"suspender",
		"mutator.Apply",
	}

	require.Len(t, recorder.events, len(expectedOrder), "events: %v", recorder.events)

	for i, event := range expectedOrder {
		assert.Equal(t, event, recorder.events[i], "at index %d", i)
	}
}
