package concepts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGraceStatusLevel(t *testing.T) {
	tests := []struct {
		name   string
		status GraceStatus
		want   int
	}{
		{"healthy", GraceStatusHealthy, 1},
		{"degraded", GraceStatusDegraded, 2},
		{"down", GraceStatusDown, 3},
		{"unknown", GraceStatus("bogus"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.Priority())
		})
	}
}
