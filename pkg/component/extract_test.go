package component

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractResourceData(t *testing.T) {
	t.Run("should call ExtractData for resources that implement DataExtractable", func(t *testing.T) {
		// Given
		res := &MockExtractableResource{}
		res.On("ExtractData").Return(nil)
		res.On("Identity").Return("mock-res")

		resources := []Resource{res}

		// When
		err := extractResourceData(resources)

		// Then
		require.NoError(t, err)
		res.AssertCalled(t, "ExtractData")
	})

	t.Run("should not call anything for resources that do not implement DataExtractable", func(t *testing.T) {
		// Given
		res := &MockResource{}
		res.On("Identity").Return("mock-res")

		resources := []Resource{res}

		// When
		err := extractResourceData(resources)

		// Then
		require.NoError(t, err)
		res.AssertNotCalled(t, "ExtractData")
	})

	t.Run("should propagate errors from ExtractData", func(t *testing.T) {
		// Given
		res := &MockExtractableResource{}
		res.On("ExtractData").Return(fmt.Errorf("extraction error"))
		res.On("Identity").Return("mock-res")

		resources := []Resource{res}

		// When
		err := extractResourceData(resources)

		// Then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to extract data from resource mock-res: extraction error")
	})

	t.Run("should handle a mix of extractable and non-extractable resources", func(t *testing.T) {
		// Given
		res1 := &MockExtractableResource{}
		res1.On("ExtractData").Return(nil)
		res1.On("Identity").Return("res1")

		res2 := &MockResource{}
		res2.On("Identity").Return("res2")

		res3 := &MockExtractableResource{}
		res3.On("ExtractData").Return(nil)
		res3.On("Identity").Return("res3")

		resources := []Resource{res1, res2, res3}

		// When
		err := extractResourceData(resources)

		// Then
		require.NoError(t, err)
		res1.AssertCalled(t, "ExtractData")
		res3.AssertCalled(t, "ExtractData")
	})

	t.Run("should return nil for an empty list of resources", func(t *testing.T) {
		// When
		err := extractResourceData(nil)

		// Then
		require.NoError(t, err)
	})
}
