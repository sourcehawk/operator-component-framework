package generic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestIsNil(t *testing.T) {
	var (
		ptr   *corev1.ConfigMap
		slice []string
		m     map[string]string
		ch    chan int
		f     func()
	)

	tests := []struct {
		name     string
		input    any
		expected bool
	}{
		{"nil any", nil, true},
		{"nil pointer", ptr, true},
		{"nil slice", slice, true},
		{"nil map", m, true},
		{"nil chan", ch, true},
		{"nil func", f, true},
		{"non-nil pointer", &corev1.ConfigMap{}, false},
		{"non-nil slice", []string{}, false},
		{"non-nil map", map[string]string{}, false},
		{"non-nil chan", make(chan int), false},
		{"non-nil func", func() {}, false},
		{"int", 1, false},
		{"string", "test", false},
		{"struct", corev1.ConfigMap{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isNil(tt.input))
		})
	}
}
