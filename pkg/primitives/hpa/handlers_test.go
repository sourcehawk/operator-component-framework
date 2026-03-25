package hpa

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
)

func TestDefaultOperationalStatusHandler(t *testing.T) {
	tests := []struct {
		name       string
		hpa        *autoscalingv2.HorizontalPodAutoscaler
		wantStatus concepts.OperationalStatus
	}{
		{
			name: "operational when ScalingActive is True",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{
							Type:    autoscalingv2.ScalingActive,
							Status:  corev1.ConditionTrue,
							Message: "the HPA was able to successfully calculate a replica count",
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusOperational,
		},
		{
			name: "pending when no conditions",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{},
			},
			wantStatus: concepts.OperationalStatusPending,
		},
		{
			name: "pending when conditions present but ScalingActive missing",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{
							Type:   autoscalingv2.AbleToScale,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusPending,
		},
		{
			name: "pending when ScalingActive is Unknown",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{
							Type:   autoscalingv2.ScalingActive,
							Status: corev1.ConditionUnknown,
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusPending,
		},
		{
			name: "failing when ScalingActive is False",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{
							Type:    autoscalingv2.ScalingActive,
							Status:  corev1.ConditionFalse,
							Message: "the HPA target is missing",
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusFailing,
		},
		{
			name: "failing when AbleToScale is False",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{
							Type:   autoscalingv2.ScalingActive,
							Status: corev1.ConditionTrue,
						},
						{
							Type:    autoscalingv2.AbleToScale,
							Status:  corev1.ConditionFalse,
							Message: "the HPA controller was unable to update the target scale",
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusFailing,
		},
		{
			name: "failing when AbleToScale is False takes precedence over ScalingActive True",
			hpa: &autoscalingv2.HorizontalPodAutoscaler{
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
						{
							Type:   autoscalingv2.AbleToScale,
							Status: corev1.ConditionFalse,
						},
						{
							Type:   autoscalingv2.ScalingActive,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			wantStatus: concepts.OperationalStatusFailing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DefaultOperationalStatusHandler(concepts.ConvergingOperationNone, tt.hpa)
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, got.Status)
		})
	}
}

func TestDefaultOperationalStatusHandler_UsesConditionMessage(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		Status: autoscalingv2.HorizontalPodAutoscalerStatus{
			Conditions: []autoscalingv2.HorizontalPodAutoscalerCondition{
				{
					Type:    autoscalingv2.ScalingActive,
					Status:  corev1.ConditionTrue,
					Message: "scaling is active and healthy",
				},
			},
		},
	}
	got, err := DefaultOperationalStatusHandler(concepts.ConvergingOperationNone, hpa)
	require.NoError(t, err)
	assert.Equal(t, "scaling is active and healthy", got.Reason)
}

func TestDefaultDeleteOnSuspendHandler(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	assert.True(t, DefaultDeleteOnSuspendHandler(hpa))
}

func TestDefaultSuspendMutationHandler(t *testing.T) {
	hpa := newTestHPA()
	m := NewMutator(hpa)
	err := DefaultSuspendMutationHandler(m)
	require.NoError(t, err)
}

func TestDefaultSuspensionStatusHandler(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	got, err := DefaultSuspensionStatusHandler(hpa)
	require.NoError(t, err)
	assert.Equal(t, concepts.SuspensionStatusSuspended, got.Status)
	assert.Equal(t, "HorizontalPodAutoscaler suspended to prevent scaling interference", got.Reason)
}
