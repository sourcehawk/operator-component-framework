package job

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestWithMetricsIdentifier covers the task builder's exposure of the metrics
// identifier, the shape shared by every task primitive.
func TestWithMetricsIdentifier(t *testing.T) {
	t.Parallel()
	newBuilder := func() *Builder {
		return NewBuilder(&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "migrate", Namespace: "default"},
		})
	}

	t.Run("defaults to empty so the framework labels by kind", func(t *testing.T) {
		t.Parallel()
		res, err := newBuilder().Build()
		require.NoError(t, err)
		assert.Empty(t, res.MetricsIdentifier())
	})

	t.Run("returns the configured identifier", func(t *testing.T) {
		t.Parallel()
		res, err := newBuilder().WithMetricsIdentifier("schema-migration").Build()
		require.NoError(t, err)
		assert.Equal(t, "schema-migration", res.MetricsIdentifier())
	})

	t.Run("rejects a blank identifier", func(t *testing.T) {
		t.Parallel()
		_, err := newBuilder().WithMetricsIdentifier(" ").Build()
		assert.EqualError(t, err, "metrics identifier cannot be blank")
	})
}
