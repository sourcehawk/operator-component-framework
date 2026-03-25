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

func (m *recordingMutator) NextFeature() {
	m.recorder.record("mutator.NextFeature")
}

func TestApplyMutationsOrder(t *testing.T) {
	recorder := &orderRecorder{}

	current := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
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
		newMutator,
		mutations,
		suspender,
	)

	require.NoError(t, err)

	expectedOrder := []string{
		"newMutator",
		"mutation1",
		"mutator.NextFeature",
		"mutator.Apply",
		"suspender",
		"mutator.Apply",
	}

	require.Len(t, recorder.events, len(expectedOrder), "events: %v", recorder.events)

	for i, event := range expectedOrder {
		assert.Equal(t, event, recorder.events[i], "at index %d", i)
	}
}

func TestApplyMutationsOrder_MultipleMutations(t *testing.T) {
	recorder := &orderRecorder{}

	current := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
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
		{
			Name:    "feat2",
			Feature: alwaysEnabled{},
			Mutate: func(_ *recordingMutator) error {
				recorder.record("mutation2")
				return nil
			},
		},
		{
			Name:    "feat3",
			Feature: alwaysEnabled{},
			Mutate: func(_ *recordingMutator) error {
				recorder.record("mutation3")
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
		newMutator,
		mutations,
		suspender,
	)

	require.NoError(t, err)

	// Each mutation uses the current scope, then NextFeature advances to the next.
	expectedOrder := []string{
		"newMutator",
		"mutation1",
		"mutator.NextFeature",
		"mutation2",
		"mutator.NextFeature",
		"mutation3",
		"mutator.NextFeature",
		"mutator.Apply",
		"suspender",
		"mutator.Apply",
	}

	require.Len(t, recorder.events, len(expectedOrder), "events: %v", recorder.events)

	for i, event := range expectedOrder {
		assert.Equal(t, event, recorder.events[i], "at index %d", i)
	}
}
