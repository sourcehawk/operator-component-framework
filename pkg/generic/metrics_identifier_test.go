package generic

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// newMetricsTestBuilder returns a valid static builder for a ConfigMap, used to
// exercise the metrics identifier without repeating the boilerplate.
func newMetricsTestBuilder() *StaticBuilder[*corev1.ConfigMap, *mockMutator] {
	return NewStaticBuilder[*corev1.ConfigMap, *mockMutator](
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "web-tls", Namespace: "default"}},
		func(cm *corev1.ConfigMap) string { return "v1/ConfigMap/" + cm.Namespace + "/" + cm.Name },
		func(*corev1.ConfigMap) *mockMutator { return &mockMutator{} },
	)
}

func TestBaseResourceMetricsIdentifier(t *testing.T) {
	t.Run("returns empty when unset so the framework applies its default", func(t *testing.T) {
		res, err := newMetricsTestBuilder().Build()
		require.NoError(t, err)
		assert.Empty(t, res.MetricsIdentifier())
	})

	t.Run("returns the configured identifier", func(t *testing.T) {
		res, err := newMetricsTestBuilder().WithMetricsIdentifier("tls").Build()
		require.NoError(t, err)
		assert.Equal(t, "tls", res.MetricsIdentifier())
	})

	t.Run("satisfies concepts.MetricsIdentifiable", func(t *testing.T) {
		res, err := newMetricsTestBuilder().Build()
		require.NoError(t, err)
		var identifiable concepts.MetricsIdentifiable = res
		assert.Empty(t, identifiable.MetricsIdentifier())
	})
}

func TestBaseBuilderRejectsBlankMetricsIdentifier(t *testing.T) {
	for _, identifier := range []string{"", " ", "\t "} {
		t.Run("rejects "+strconv.Quote(identifier), func(t *testing.T) {
			_, err := newMetricsTestBuilder().WithMetricsIdentifier(identifier).Build()
			assert.EqualError(t, err, "metrics identifier cannot be blank")
		})
	}
}
