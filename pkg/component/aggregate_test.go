package component_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
)

// stagedComponent describes one component participating in an aggregation test:
// the component to build and, unless absent is set, the condition it has already
// staged on the owner.
type stagedComponent struct {
	name    string
	absent  bool
	status  metav1.ConditionStatus
	reason  component.Status
	message string
}

// conditionType returns the condition type owned by the staged component.
func (s stagedComponent) conditionType() component.ConditionType {
	return component.ConditionType(s.name + "Ready")
}

// stage builds one component per entry and writes its condition onto a fresh
// owner at the given generation. Entries marked absent get a component but no
// condition, which is how a component that has never reconciled looks.
func stage(t *testing.T, generation int64, staged ...stagedComponent) (*component.MockOperatorCRD, []*component.Component) {
	t.Helper()

	owner := &component.MockOperatorCRD{}
	owner.Generation = generation

	comps := make([]*component.Component, 0, len(staged))
	for _, s := range staged {
		comp, err := component.NewComponentBuilder().
			WithName(s.name).
			WithConditionType(s.conditionType()).
			Build()
		require.NoError(t, err)
		comps = append(comps, comp)

		if s.absent {
			continue
		}

		meta.SetStatusCondition(owner.GetStatusConditions(), metav1.Condition{
			Type:    string(s.conditionType()),
			Status:  s.status,
			Reason:  string(s.reason),
			Message: s.message,
		})
	}

	return owner, comps
}

func TestAggregate(t *testing.T) {
	const aggregateType = component.ConditionType("Ready")

	healthy := func(name string) stagedComponent {
		return stagedComponent{
			name: name, status: metav1.ConditionTrue,
			reason: component.Healthy, message: "Component is healthy.",
		}
	}
	suspended := func(name string) stagedComponent {
		return stagedComponent{
			name: name, status: metav1.ConditionTrue,
			reason: component.Suspended, message: "Component is suspended.",
		}
	}
	disabled := func(name string) stagedComponent {
		return stagedComponent{
			name: name, status: metav1.ConditionTrue,
			reason: component.Disabled, message: "Component is disabled.",
		}
	}
	failing := func(name, message string) stagedComponent {
		return stagedComponent{
			name: name, status: metav1.ConditionFalse,
			reason: component.AliveFailing, message: message,
		}
	}

	tests := []struct {
		name     string
		staged   []stagedComponent
		expected component.Condition
	}{
		{
			name:   "all components healthy aggregates to True with reason Healthy",
			staged: []stagedComponent{healthy("broker"), healthy("gateway")},
			expected: component.Condition{
				Status:  metav1.ConditionTrue,
				Reason:  string(component.Healthy),
				Message: "broker: Component is healthy.",
			},
		},
		{
			name:   "a failing component makes a healthy aggregate False",
			staged: []stagedComponent{healthy("broker"), failing("gateway", "Pods are crash looping.")},
			expected: component.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(component.AliveFailing),
				Message: "gateway: Pods are crash looping.",
			},
		},
		{
			name:   "a failing component outranks a suspended one despite lower priority",
			staged: []stagedComponent{suspended("broker"), failing("gateway", "Pods are crash looping.")},
			expected: component.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(component.AliveFailing),
				Message: "gateway: Pods are crash looping.",
			},
		},
		{
			name:   "all components suspended aggregates to True with reason Suspended",
			staged: []stagedComponent{suspended("broker"), suspended("gateway")},
			expected: component.Condition{
				Status:  metav1.ConditionTrue,
				Reason:  string(component.Suspended),
				Message: "broker: Component is suspended.",
			},
		},
		{
			name:   "a suspended component governs the reason of an otherwise healthy aggregate",
			staged: []stagedComponent{suspended("broker"), healthy("gateway")},
			expected: component.Condition{
				Status:  metav1.ConditionTrue,
				Reason:  string(component.Suspended),
				Message: "broker: Component is suspended.",
			},
		},
		{
			name:   "priority not argument order governs the reason within the True group",
			staged: []stagedComponent{healthy("broker"), suspended("gateway")},
			expected: component.Condition{
				Status:  metav1.ConditionTrue,
				Reason:  string(component.Suspended),
				Message: "gateway: Component is suspended.",
			},
		},
		{
			name:   "a disabled component governs the reason and stays True",
			staged: []stagedComponent{healthy("broker"), disabled("gateway")},
			expected: component.Condition{
				Status:  metav1.ConditionTrue,
				Reason:  string(component.Disabled),
				Message: "gateway: Component is disabled.",
			},
		},
		{
			name:   "a component that has never reconciled makes the aggregate False and Unknown",
			staged: []stagedComponent{healthy("broker"), {name: "gateway", absent: true}},
			expected: component.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(component.Unknown),
				Message: "gateway: Component has not been reconciled yet.",
			},
		},
		{
			name: "an errored component governs the reason",
			staged: []stagedComponent{
				healthy("broker"),
				{
					name: "gateway", status: metav1.ConditionFalse,
					reason: component.Error, message: "failed to apply Deployment.",
				},
			},
			expected: component.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(component.Error),
				Message: "gateway: failed to apply Deployment.",
			},
		},
		{
			name:   "no components aggregates to False with reason Unknown",
			staged: nil,
			expected: component.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(component.Unknown),
				Message: "No components were aggregated.",
			},
		},
		{
			name:   "argument order breaks a priority tie within the non-True group",
			staged: []stagedComponent{failing("broker", "Broker is down."), failing("gateway", "Gateway is down.")},
			expected: component.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(component.AliveFailing),
				Message: "broker: Broker is down.",
			},
		},
		{
			name:   "argument order breaks a priority tie within the True group",
			staged: []stagedComponent{suspended("broker"), suspended("gateway"), healthy("worker")},
			expected: component.Condition{
				Status:  metav1.ConditionTrue,
				Reason:  string(component.Suspended),
				Message: "broker: Component is suspended.",
			},
		},
		{
			name:   "a single healthy component aggregates to True",
			staged: []stagedComponent{healthy("broker")},
			expected: component.Condition{
				Status:  metav1.ConditionTrue,
				Reason:  string(component.Healthy),
				Message: "broker: Component is healthy.",
			},
		},
		{
			name: "a single converging component aggregates to False",
			staged: []stagedComponent{{
				name: "broker", status: metav1.ConditionFalse,
				reason: component.AliveCreating, message: "Deployment is being created.",
			}},
			expected: component.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(component.AliveCreating),
				Message: "broker: Deployment is being created.",
			},
		},
		{
			name: "a governing condition without a message reduces to the component name",
			staged: []stagedComponent{{
				name: "broker", status: metav1.ConditionFalse, reason: component.AliveFailing,
			}},
			expected: component.Condition{
				Status:  metav1.ConditionFalse,
				Reason:  string(component.AliveFailing),
				Message: "broker",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, comps := stage(t, 3, tt.staged...)

			got := component.Aggregate(aggregateType, owner, comps...)

			assert.Equal(t, string(aggregateType), got.Type)
			assert.Equal(t, tt.expected.Status, got.Status)
			assert.Equal(t, tt.expected.Reason, got.Reason)
			assert.Equal(t, tt.expected.Message, got.Message)
			assert.Equal(t, int64(3), got.ObservedGeneration)
		})
	}
}

func TestAggregateObservedGenerationTracksOwner(t *testing.T) {
	owner, comps := stage(t, 9, stagedComponent{
		name: "broker", status: metav1.ConditionTrue,
		reason: component.Healthy, message: "Component is healthy.",
	})

	assert.Equal(t, int64(9), component.Aggregate("Ready", owner, comps...).ObservedGeneration)

	owner.Generation = 10
	assert.Equal(t, int64(10), component.Aggregate("Ready", owner, comps...).ObservedGeneration)
}

func TestAggregateWithoutComponentsTracksOwnerGeneration(t *testing.T) {
	owner, _ := stage(t, 4)

	got := component.Aggregate("Ready", owner)

	assert.Equal(t, int64(4), got.ObservedGeneration)
	assert.Equal(t, metav1.ConditionFalse, got.Status)
}

func TestAggregateResultIsStageable(t *testing.T) {
	owner, comps := stage(t, 1, stagedComponent{
		name: "broker", status: metav1.ConditionTrue,
		reason: component.Healthy, message: "Component is healthy.",
	})

	got := component.Aggregate("Ready", owner, comps...)
	meta.SetStatusCondition(owner.GetStatusConditions(), metav1.Condition(got))

	staged := meta.FindStatusCondition(*owner.GetStatusConditions(), "Ready")
	require.NotNil(t, staged)
	assert.Equal(t, metav1.ConditionTrue, staged.Status)
	assert.Equal(t, string(component.Healthy), staged.Reason)
}
