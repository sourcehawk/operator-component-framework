package rolebinding

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
		rb          *rbacv1.RoleBinding
		expectedErr string
	}{
		{
			name:        "nil rolebinding",
			rb:          nil,
			expectedErr: "object cannot be nil",
		},
		{
			name: "empty name",
			rb: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Namespace: "test-ns"},
			},
			expectedErr: "object name cannot be empty",
		},
		{
			name: "empty namespace",
			rb: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test-rb"},
			},
			expectedErr: "object namespace cannot be empty",
		},
		{
			name: "empty roleRef",
			rb: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
			},
			expectedErr: "roleRef must have non-empty APIGroup, Kind, and Name",
		},
		{
			name: "partial roleRef missing kind",
			rb: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Name:     "my-role",
				},
			},
			expectedErr: "roleRef must have non-empty APIGroup, Kind, and Name",
		},
		{
			name: "valid rolebinding",
			rb: &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
				RoleRef: rbacv1.RoleRef{
					APIGroup: "rbac.authorization.k8s.io",
					Kind:     "Role",
					Name:     "my-role",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := NewBuilder(tt.rb).Build()
			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, res)
			} else {
				require.NoError(t, err)
				require.NotNil(t, res)
				assert.Equal(t, "rbac.authorization.k8s.io/v1/RoleBinding/test-ns/test-rb", res.Identity())
			}
		})
	}
}

func testRoleRef() rbacv1.RoleRef {
	return rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: "my-role"}
}

func TestBuilder_WithMutation(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
		RoleRef:    testRoleRef(),
	}
	res, err := NewBuilder(rb).
		WithMutation(Mutation{Name: "test-mutation"}).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.Mutations, 1)
	assert.Equal(t, "test-mutation", res.base.Mutations[0].Name)
}

func TestBuilder_WithDataExtractor(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
		RoleRef:    testRoleRef(),
	}
	called := false
	extractor := func(_ rbacv1.RoleBinding) error {
		called = true
		return nil
	}
	res, err := NewBuilder(rb).
		WithDataExtractor(extractor).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 1)
	require.NoError(t, res.base.DataExtractors[0](&rbacv1.RoleBinding{}))
	assert.True(t, called)
}

func TestBuilder_WithDataExtractor_Nil(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
		RoleRef:    testRoleRef(),
	}
	res, err := NewBuilder(rb).
		WithDataExtractor(nil).
		Build()
	require.NoError(t, err)
	assert.Len(t, res.base.DataExtractors, 0)
}

func TestBuilder_WithDataExtractor_ErrorPropagated(t *testing.T) {
	t.Parallel()
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "test-rb", Namespace: "test-ns"},
		RoleRef:    testRoleRef(),
	}
	res, err := NewBuilder(rb).
		WithDataExtractor(func(_ rbacv1.RoleBinding) error {
			return errors.New("extractor error")
		}).
		Build()
	require.NoError(t, err)
	err = res.base.DataExtractors[0](&rbacv1.RoleBinding{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extractor error")
}

func TestExtractIntoDeclaredExtraction(t *testing.T) {
	t.Parallel()
	cell := concepts.NewData[string]("team-label")
	builder := NewBuilder(&rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "default", Labels: map[string]string{"team": "platform"}},
		RoleRef:    testRoleRef(),
	})
	ExtractInto(builder, cell, func(o rbacv1.RoleBinding) (string, error) {
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
	builder := NewBuilder(&rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rb", Namespace: "default"},
		RoleRef:    testRoleRef(),
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
