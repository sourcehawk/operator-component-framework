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

func TestApplyBaselineAndFlavors(t *testing.T) {
	defaultApp := func(current, desired *corev1.ConfigMap) error {
		current.Data = desired.Data
		return nil
	}

	tests := []struct {
		name       string
		current    *corev1.ConfigMap
		desired    *corev1.ConfigMap
		defaultApp FieldApplicator[*corev1.ConfigMap]
		customApp  FieldApplicator[*corev1.ConfigMap]
		flavors    []FieldApplicationFlavor[*corev1.ConfigMap]
		wantErr    bool
		validate   func(*testing.T, *corev1.ConfigMap)
	}{
		{
			name:       "use default applicator",
			current:    &corev1.ConfigMap{},
			desired:    &corev1.ConfigMap{Data: map[string]string{"foo": "bar"}},
			defaultApp: defaultApp,
			validate: func(t *testing.T, cm *corev1.ConfigMap) {
				assert.Equal(t, "bar", cm.Data["foo"])
			},
		},
		{
			name:       "use custom applicator",
			current:    &corev1.ConfigMap{},
			desired:    &corev1.ConfigMap{Data: map[string]string{"foo": "bar"}},
			defaultApp: defaultApp,
			customApp: func(current, _ *corev1.ConfigMap) error {
				current.Data = map[string]string{"custom": "value"}
				return nil
			},
			validate: func(t *testing.T, cm *corev1.ConfigMap) {
				assert.Equal(t, "value", cm.Data["custom"])
				assert.NotContains(t, cm.Data, "foo")
			},
		},
		{
			name: "run flavors",
			current: &corev1.ConfigMap{
				Data: map[string]string{"preserved": "old"},
			},
			desired:    &corev1.ConfigMap{Data: map[string]string{"foo": "bar"}},
			defaultApp: defaultApp,
			flavors: []FieldApplicationFlavor[*corev1.ConfigMap]{
				func(applied, current, _ *corev1.ConfigMap) error {
					if val, ok := current.Data["preserved"]; ok {
						if applied.Data == nil {
							applied.Data = make(map[string]string)
						}
						applied.Data["preserved"] = val
					}
					return nil
				},
			},
			validate: func(t *testing.T, cm *corev1.ConfigMap) {
				assert.Equal(t, "bar", cm.Data["foo"])
				assert.Equal(t, "old", cm.Data["preserved"])
			},
		},
		{
			name:    "no applicator",
			current: &corev1.ConfigMap{},
			desired: &corev1.ConfigMap{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := applyBaselineAndFlavors(tt.current, tt.desired, tt.defaultApp, tt.customApp, tt.flavors)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, got)
			}
		})
	}
}
