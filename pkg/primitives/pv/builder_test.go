package pv

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder_Build_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pv          *corev1.PersistentVolume
		expectedErr string
	}{
		{
			name:        "nil persistent volume",
			pv:          nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			pv: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "namespace set on cluster-scoped resource",
			pv: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pv", Namespace: "should-not-be-set"},
			},
			expectedErr: "cluster-scoped object must not have a namespace",
		},
		{
			name: "valid persistent volume",
			pv: &corev1.PersistentVolume{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.pv).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "v1/PersistentVolume/test-pv", res.Identity())
			}
		})
	}
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
	}
	res, err := NewBuilder(pv).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithCustomOperationalStatus(t *testing.T) {
	t.Parallel()
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
	}
	handler := func(_ concepts.ConvergingOperation, _ *corev1.PersistentVolume) (concepts.OperationalStatusWithReason, error) {
		return concepts.OperationalStatusWithReason{Status: concepts.OperationalStatusOperational}, nil
	}
	res, err := NewBuilder(pv).
		WithCustomOperationalStatus(handler).
		Build()
	require.NoError(t, err)
	require.NotNil(t, res.base.OperationalStatusHandler)
	status, err := res.base.OperationalStatusHandler(concepts.ConvergingOperationNone, nil)
	require.NoError(t, err)
	assert.Equal(t, concepts.OperationalStatusOperational, status.Status)
}

func TestExtractIntoDeclaredExtraction(t *testing.T) {
	t.Parallel()
	cell := concepts.NewData[string]("team-label")
	builder := NewBuilder(&corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv", Labels: map[string]string{"team": "platform"}},
	})
	ExtractInto(builder, cell, func(o corev1.PersistentVolume) (string, error) {
		return o.Labels["team"], nil
	})

	res, err := builder.Build()
	require.NoError(t, err)

	produced := res.ProducedData()
	require.Len(t, produced, 1)
	assert.Equal(t, "team-label", produced[0].Name())

	require.NoError(t, res.ExtractData())
	v, ok := cell.Get()
	assert.True(t, ok)
	assert.Equal(t, "platform", v)
}

func TestWithDataGuardAndOptionalDataDeclarations(t *testing.T) {
	t.Parallel()
	guarded := concepts.NewData[string]("db-host")
	optional := concepts.NewData[string]("db-port")
	builder := NewBuilder(&corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "pv"},
	}).WithDataGuard(guarded).WithOptionalData(optional)

	res, err := builder.Build()
	require.NoError(t, err)

	consumed := res.ConsumedData()
	require.Len(t, consumed, 2)
	assert.Equal(t, "db-host", consumed[0].Cell.Name())
	assert.False(t, consumed[0].Optional)
	assert.Equal(t, "db-port", consumed[1].Cell.Name())
	assert.True(t, consumed[1].Optional)

	status, err := res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusBlocked, status.Status)
	assert.Equal(t, `waiting for data "db-host"`, status.Reason)

	guarded.Set("postgres.default.svc")
	status, err = res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusUnblocked, status.Status)
}
