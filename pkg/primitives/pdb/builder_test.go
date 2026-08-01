package pdb

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder_Build_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pdb         *policyv1.PodDisruptionBudget
		expectedErr string
	}{
		{
			name:        "nil pdb",
			pdb:         nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			pdb: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "empty namespace",
			pdb: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pdb"},
			},
			expectedErr: "object namespace cannot be empty",
		},
		{
			name: "valid pdb",
			pdb: &policyv1.PodDisruptionBudget{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pdb", Namespace: "test-ns"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.pdb).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "policy/v1/PodDisruptionBudget/test-ns/test-pdb", res.Identity())
			}
		})
	}
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	p := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pdb", Namespace: "test-ns"},
	}
	res, err := NewBuilder(p).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestExtractIntoDeclaredExtraction(t *testing.T) {
	t.Parallel()
	cell := concepts.NewData[string]("team-label")
	builder := NewBuilder(&policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "default", Labels: map[string]string{"team": "platform"}},
	})
	ExtractInto(builder, cell, func(o policyv1.PodDisruptionBudget) (string, error) {
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
	builder := NewBuilder(&policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "pdb", Namespace: "default"},
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
