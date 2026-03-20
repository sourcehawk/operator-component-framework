package concepts

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSuspensionStatusLevel(t *testing.T) {
	tests := []struct {
		name   string
		status SuspensionStatus
		want   int
	}{
		{"suspended", SuspensionStatusSuspended, 1},
		{"suspending", SuspensionStatusSuspending, 2},
		{"pending", SuspensionStatusPending, 3},
		{"unknown", SuspensionStatus("bogus"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.Priority())
		})
	}
}
