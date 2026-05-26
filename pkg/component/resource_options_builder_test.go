package component

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type enabledFeature struct{}

func (f *enabledFeature) Enabled() (bool, error) { return true, nil }

type disabledFeature struct{}

func (f *disabledFeature) Enabled() (bool, error) { return false, nil }

type errorFeature struct{}

func (f *errorFeature) Enabled() (bool, error) { return false, fmt.Errorf("feature evaluation failed") }

func TestResourceOptionsBuilder_Build(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (ResourceOptions, error)
		want    ResourceOptions
		wantErr bool
	}{
		{
			name: "default build produces zero-value options",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().Build()
			},
			want: ResourceOptions{},
		},
		{
			name: "feature enabled creates resource normally",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().WithFeatureGate(&enabledFeature{}).Build()
			},
			want: ResourceOptions{},
		},
		{
			name: "feature disabled marks resource for deletion",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().WithFeatureGate(&disabledFeature{}).Build()
			},
			want: ResourceOptions{Delete: true},
		},
		{
			name: "feature error propagates",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().WithFeatureGate(&errorFeature{}).Build()
			},
			wantErr: true,
		},
		{
			name: "nil feature treated as enabled",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().WithFeatureGate(nil).Build()
			},
			want: ResourceOptions{},
		},
		{
			name: "single When condition true creates resource",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().When(true).Build()
			},
			want: ResourceOptions{},
		},
		{
			name: "single When condition false marks for deletion",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().When(false).Build()
			},
			want: ResourceOptions{Delete: true},
		},
		{
			name: "multiple When conditions all true creates resource",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					When(true).
					When(true).
					When(true).
					Build()
			},
			want: ResourceOptions{},
		},
		{
			name: "multiple When conditions one false marks for deletion",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					When(true).
					When(false).
					When(true).
					Build()
			},
			want: ResourceOptions{Delete: true},
		},
		{
			name: "feature enabled with one When condition false marks for deletion",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					WithFeatureGate(&enabledFeature{}).
					When(false).
					Build()
			},
			want: ResourceOptions{Delete: true},
		},
		{
			name: "feature disabled ignores When conditions",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					WithFeatureGate(&disabledFeature{}).
					When(true).
					Build()
			},
			want: ResourceOptions{Delete: true},
		},
		{
			name: "auxiliary sets participation mode",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().Auxiliary().Build()
			},
			want: ResourceOptions{ParticipationMode: ParticipationModeAuxiliary},
		},
		{
			name: "auxiliary preserved when feature disabled",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					WithFeatureGate(&disabledFeature{}).
					Auxiliary().
					Build()
			},
			want: ResourceOptions{Delete: true, ParticipationMode: ParticipationModeAuxiliary},
		},
		{
			name: "read-only sets flag",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().ReadOnly().Build()
			},
			want: ResourceOptions{ReadOnly: true},
		},
		{
			name: "read-only forced false when feature disabled",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					WithFeatureGate(&disabledFeature{}).
					ReadOnly().
					Build()
			},
			want: ResourceOptions{Delete: true, ReadOnly: false},
		},
		{
			name: "suppress grace inconsistency warning sets flag",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().SuppressGraceInconsistencyWarning().Build()
			},
			want: ResourceOptions{SuppressGraceInconsistencyWarning: true},
		},
		{
			name: "block on absence sets flag alongside read-only",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().ReadOnly().BlockOnAbsence().Build()
			},
			want: ResourceOptions{ReadOnly: true, BlockOnAbsence: true},
		},
		{
			name: "block on absence preserved when deletion forced",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					WithFeatureGate(&disabledFeature{}).
					ReadOnly().
					BlockOnAbsence().
					Build()
			},
			want: ResourceOptions{Delete: true, ReadOnly: false, BlockOnAbsence: true},
		},
		{
			name: "ignore if absent sets flag alongside read-only",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().ReadOnly().IgnoreIfAbsent().Build()
			},
			want: ResourceOptions{ReadOnly: true, IgnoreIfAbsent: true},
		},
		{
			name: "ignore if absent preserved when deletion forced",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					WithFeatureGate(&disabledFeature{}).
					ReadOnly().
					IgnoreIfAbsent().
					Build()
			},
			want: ResourceOptions{Delete: true, ReadOnly: false, IgnoreIfAbsent: true},
		},
		{
			name: "last WithFeatureGate wins",
			build: func() (ResourceOptions, error) {
				return NewResourceOptionsBuilder().
					WithFeatureGate(&disabledFeature{}).
					WithFeatureGate(&enabledFeature{}).
					Build()
			},
			want: ResourceOptions{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.build()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResourceOptionsFor(t *testing.T) {
	t.Run("enabled feature creates resource", func(t *testing.T) {
		opts, err := ResourceOptionsFor(&enabledFeature{})
		require.NoError(t, err)
		assert.Equal(t, ResourceOptions{}, opts)
	})

	t.Run("disabled feature marks for deletion", func(t *testing.T) {
		opts, err := ResourceOptionsFor(&disabledFeature{})
		require.NoError(t, err)
		assert.Equal(t, ResourceOptions{Delete: true}, opts)
	})

	t.Run("nil feature treated as enabled", func(t *testing.T) {
		opts, err := ResourceOptionsFor(nil)
		require.NoError(t, err)
		assert.Equal(t, ResourceOptions{}, opts)
	})

	t.Run("feature error propagates", func(t *testing.T) {
		_, err := ResourceOptionsFor(&errorFeature{})
		require.Error(t, err)
	})
}
