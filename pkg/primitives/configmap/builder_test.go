package configmap

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
		cm          *corev1.ConfigMap
		expectedErr string
	}{
		{
			name:        "nil configmap",
			cm:          nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "empty namespace",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cm"},
			},
			expectedErr: "object namespace cannot be empty",
		},
		{
			name: "valid configmap",
			cm: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "test-ns"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.cm).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "v1/ConfigMap/test-ns/test-cm", res.Identity())
			}
		})
	}
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "test-ns"},
	}
	res, err := NewBuilder(cm).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestExtractIntoDeclaredExtraction(t *testing.T) {
	t.Parallel()
	cell := concepts.NewData[string]("db-host")
	builder := NewBuilder(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
		Data:       map[string]string{"db-host": "postgres.default.svc"},
	})
	ExtractInto(builder, cell, func(cm corev1.ConfigMap) (string, error) {
		return cm.Data["db-host"], nil
	})

	res, err := builder.Build()
	require.NoError(t, err)

	produced := res.ProducedData()
	require.Len(t, produced, 1)
	assert.Equal(t, "db-host", produced[0].Name())

	require.NoError(t, res.ExtractData())
	v, ok := cell.Get()
	assert.True(t, ok)
	assert.Equal(t, "postgres.default.svc", v)
}

func TestWithDataGuardAndOptionalDataDeclarations(t *testing.T) {
	t.Parallel()
	guarded := concepts.NewData[string]("db-host")
	optional := concepts.NewData[string]("db-port")
	builder := NewBuilder(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
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
