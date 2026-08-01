package clusterrole

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuilder_Build_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cr          *rbacv1.ClusterRole
		expectedErr string
	}{
		{
			name:        "nil clusterrole",
			cr:          nil,
			expectedErr: "object cannot be nil",
		},
		{
			name:        "empty name",
			cr:          &rbacv1.ClusterRole{},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "valid clusterrole",
			cr: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
			},
		},
		{
			name: "rejects namespace on cluster-scoped resource",
			cr: &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cr", Namespace: "oops"},
			},
			expectedErr: "cluster-scoped object must not have a namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.cr).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "rbac.authorization.k8s.io/v1/ClusterRole/test-cr", res.Identity())
			}
		})
	}
}

func TestBuilder_Build_NoNamespaceRequired(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-scoped"},
	}
	res, err := NewBuilder(cr).Build()
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, "rbac.authorization.k8s.io/v1/ClusterRole/cluster-scoped", res.Identity())
}

func TestBuilder_Build_DoesNotSetNamespace(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	_, err := NewBuilder(cr).Build()
	require.NoError(t, err)
	assert.Empty(t, cr.Namespace, "namespace should not be set by Build")
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	res, err := NewBuilder(cr).
		WithMutation(Mutation{
			Name: "test-mutation",
			Mutate: func(m *Mutator) error {
				m.AddRule(rbacv1.PolicyRule{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get"},
				})
				return nil
			},
		}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	called := false
	extractor := func(_ rbacv1.ClusterRole) error {
		called = true
		return nil
	}
	res, err := NewBuilder(cr).
		WithDataExtractor(extractor).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 1)
	require.NoError(t, res.base.DataExtractors[0](&rbacv1.ClusterRole{}))
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_Nil(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	res, err := NewBuilder(cr).
		WithDataExtractor(nil).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 0)
}

func TestBuilder_WithDataExtractor_ErrorPropagated(t *testing.T) {
	t.Parallel()
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cr"},
	}
	res, err := NewBuilder(cr).
		WithDataExtractor(func(_ rbacv1.ClusterRole) error {
			return errors.New("extractor error")
		}).
		Build()
	require.NoError(t, err)
	err = res.base.DataExtractors[0](&rbacv1.ClusterRole{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor error")
}

func TestExtractIntoDeclaredExtraction(t *testing.T) {
	t.Parallel()
	cell := concepts.NewData[string]("team-label")
	builder := NewBuilder(&rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cr", Labels: map[string]string{"team": "platform"}},
	})
	ExtractInto(builder, cell, func(o rbacv1.ClusterRole) (string, error) {
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
	builder := NewBuilder(&rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "cr"},
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
