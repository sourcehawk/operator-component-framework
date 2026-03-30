package component

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// mockPrerequisite is a test Prerequisite that returns a fixed result.
type mockPrerequisite struct {
	result PrerequisiteResult
	err    error
}

func (m *mockPrerequisite) Check(_ ReconcileContext) (PrerequisiteResult, error) {
	return m.result, m.err
}

func TestDependsOn(t *testing.T) {
	t.Parallel()

	condType := ConditionType("DepReady")

	t.Run("returns Met when condition is True", func(t *testing.T) {
		t.Parallel()

		owner := &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{Name: "test-owner"},
			Status: MockStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(condType),
						Status: metav1.ConditionTrue,
						Reason: "Healthy",
					},
				},
			},
		}

		prereq := DependsOn(condType)
		rec := ReconcileContext{Owner: owner}
		result, err := prereq.Check(rec)

		require.NoError(t, err)
		assert.Equal(t, PrerequisiteStatusMet, result.Status)
	})

	t.Run("returns NotMet when condition is False", func(t *testing.T) {
		t.Parallel()

		owner := &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{Name: "test-owner"},
			Status: MockStatus{
				Conditions: []metav1.Condition{
					{
						Type:    string(condType),
						Status:  metav1.ConditionFalse,
						Reason:  "Creating",
						Message: "Still creating resources",
					},
				},
			},
		}

		prereq := DependsOn(condType)
		rec := ReconcileContext{Owner: owner}
		result, err := prereq.Check(rec)

		require.NoError(t, err)
		assert.Equal(t, PrerequisiteStatusNotMet, result.Status)
		assert.Contains(t, result.Reason, string(condType))
		assert.Contains(t, result.Reason, "False")
	})

	t.Run("returns NotMet when condition does not exist", func(t *testing.T) {
		t.Parallel()

		owner := &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{Name: "test-owner"},
		}

		prereq := DependsOn(condType)
		rec := ReconcileContext{Owner: owner}
		result, err := prereq.Check(rec)

		require.NoError(t, err)
		assert.Equal(t, PrerequisiteStatusNotMet, result.Status)
		assert.Contains(t, result.Reason, string(condType))
		assert.Contains(t, result.Reason, "not found")
	})
}

func TestEvaluatePrerequisites(t *testing.T) {
	t.Parallel()

	rec := ReconcileContext{}

	t.Run("returns Met when all prerequisites pass", func(t *testing.T) {
		t.Parallel()

		comp := &Component{
			prerequisites: []Prerequisite{
				&mockPrerequisite{result: PrerequisiteResult{Status: PrerequisiteStatusMet}},
				&mockPrerequisite{result: PrerequisiteResult{Status: PrerequisiteStatusMet}},
			},
		}

		result, err := comp.evaluatePrerequisites(rec)
		require.NoError(t, err)
		assert.Equal(t, PrerequisiteStatusMet, result.Status)
	})

	t.Run("returns first NotMet result", func(t *testing.T) {
		t.Parallel()

		comp := &Component{
			prerequisites: []Prerequisite{
				&mockPrerequisite{result: PrerequisiteResult{Status: PrerequisiteStatusMet}},
				&mockPrerequisite{result: PrerequisiteResult{Status: PrerequisiteStatusNotMet, Reason: "dep not ready"}},
				&mockPrerequisite{result: PrerequisiteResult{Status: PrerequisiteStatusNotMet, Reason: "should not reach"}},
			},
		}

		result, err := comp.evaluatePrerequisites(rec)
		require.NoError(t, err)
		assert.Equal(t, PrerequisiteStatusNotMet, result.Status)
		assert.Equal(t, "dep not ready", result.Reason)
	})

	t.Run("returns error from prerequisite", func(t *testing.T) {
		t.Parallel()

		comp := &Component{
			prerequisites: []Prerequisite{
				&mockPrerequisite{err: errors.New("check failed")},
			},
		}

		_, err := comp.evaluatePrerequisites(rec)
		require.Error(t, err)
		assert.Equal(t, "check failed", err.Error())
	})
}

func TestPrerequisiteBarrierActive(t *testing.T) {
	t.Parallel()

	comp := &Component{}

	tests := []struct {
		name     string
		reason   string
		expected bool
	}{
		{"active when Unknown", string(Unknown), true},
		{"active when PrerequisiteNotMet", string(PrerequisiteNotMet), true},
		{"inactive when Healthy", string(Healthy), false},
		{"inactive when Creating", string(AliveCreating), false},
		{"inactive when Suspended", string(Suspended), false},
		{"active when Disabled", string(Disabled), true},
		{"inactive when Error", string(Error), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cond := Condition{Reason: tt.reason}
			assert.Equal(t, tt.expected, comp.prerequisiteBarrierActive(cond))
		})
	}
}
