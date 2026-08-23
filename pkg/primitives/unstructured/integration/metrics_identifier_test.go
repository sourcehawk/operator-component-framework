package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithMetricsIdentifier covers the integration builder's exposure of the
// metrics identifier. The unstructured builders have no fixed kind, so the
// framework's kind default is resolved from the object's GVK at apply time.
func TestWithMetricsIdentifier(t *testing.T) {
	t.Run("defaults to empty so the framework labels by kind", func(t *testing.T) {
		res, err := withRequiredHandlers(NewBuilder(validObject())).Build()
		require.NoError(t, err)
		assert.Empty(t, res.MetricsIdentifier())
	})

	t.Run("returns the configured identifier", func(t *testing.T) {
		res, err := withRequiredHandlers(NewBuilder(validObject())).
			WithMetricsIdentifier("gateway").Build()
		require.NoError(t, err)
		assert.Equal(t, "gateway", res.MetricsIdentifier())
	})

	t.Run("rejects a blank identifier", func(t *testing.T) {
		_, err := withRequiredHandlers(NewBuilder(validObject())).
			WithMetricsIdentifier(" ").Build()
		assert.EqualError(t, err, "metrics identifier cannot be blank")
	})
}
