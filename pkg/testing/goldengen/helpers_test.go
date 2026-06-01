package goldengen_test

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// staticGate is a feature.Gate with a fixed outcome for tests. A nil *staticGate
// is never used; pass a value to gate a mutation off.
type staticGate struct {
	enabled bool
	err     error
}

func (g staticGate) Enabled() (bool, error) { return g.enabled, g.err }

// testScheme returns a scheme with the core and apps Kubernetes types registered,
// sufficient to serialize StatefulSets and ConfigMaps.
func testScheme() *runtime.Scheme {
	return clientgoscheme.Scheme
}

// baseStatefulSet returns a minimal StatefulSet usable as the desired object for a
// statefulset builder in tests.
func baseStatefulSet() *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "StatefulSet"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "db"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "db", Image: "db:1.0.0"}},
				},
			},
		},
	}
}

// buildStatefulSetWith builds a statefulset.Resource with the supplied mutations.
func buildStatefulSetWith(t *testing.T, mutations ...statefulset.Mutation) *statefulset.Resource {
	t.Helper()
	res, err := statefulset.NewBuilder(baseStatefulSet()).
		WithMutation(mutations...).
		Build()
	require.NoError(t, err)
	return res
}

// namedStatefulSet builds a statefulset.Resource with a distinct name so it can
// coexist with others in a single component.
func namedStatefulSet(t *testing.T, name string) *statefulset.Resource {
	t.Helper()
	sts := baseStatefulSet()
	sts.Name = name
	res, err := statefulset.NewBuilder(sts).Build()
	require.NoError(t, err)
	return res
}

// buildComponentWith builds a component managing the supplied statefulset
// resources. Each resource must have a distinct identity.
func buildComponentWith(t *testing.T, resources ...*statefulset.Resource) *component.Component {
	t.Helper()
	b := component.NewComponentBuilder().
		WithName("test").
		WithConditionType("TestReady")
	for _, r := range resources {
		b = b.WithResource(r)
	}
	c, err := b.Build()
	require.NoError(t, err)
	return c
}
