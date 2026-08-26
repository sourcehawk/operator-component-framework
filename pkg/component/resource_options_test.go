package component

import (
	"fmt"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type enabledFeature struct{}

func (f *enabledFeature) Enabled() (bool, error) { return true, nil }

type disabledFeature struct{}

func (f *disabledFeature) Enabled() (bool, error) { return false, nil }

type errorFeature struct{}

func (f *errorFeature) Enabled() (bool, error) { return false, fmt.Errorf("feature evaluation failed") }

func TestResolveResourceOptions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		opts    []ResourceOption
		want    resourceOptions
		wantErr bool
	}{
		{
			name: "nil options are ignored",
			opts: []ResourceOption{nil, ReadOnly(), nil},
			want: resourceOptions{ReadOnly: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "no options produces required defaults",
			opts: nil,
			want: resourceOptions{ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "enabled gate manages resource normally",
			opts: []ResourceOption{GatedBy(&enabledFeature{})},
			want: resourceOptions{ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "disabled gate marks resource for deletion",
			opts: []ResourceOption{GatedBy(&disabledFeature{})},
			want: resourceOptions{Delete: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name:    "gate error propagates",
			opts:    []ResourceOption{GatedBy(&errorFeature{})},
			wantErr: true,
		},
		{
			name: "nil gate treated as enabled",
			opts: []ResourceOption{GatedBy(nil)},
			want: resourceOptions{ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "DeleteWhen false keeps resource",
			opts: []ResourceOption{DeleteWhen(false)},
			want: resourceOptions{ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "DeleteWhen true marks for deletion",
			opts: []ResourceOption{DeleteWhen(true)},
			want: resourceOptions{Delete: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "Delete marks for deletion",
			opts: []ResourceOption{Delete()},
			want: resourceOptions{Delete: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "multiple DeleteWhen all false keeps resource",
			opts: []ResourceOption{DeleteWhen(false), DeleteWhen(false), DeleteWhen(false)},
			want: resourceOptions{ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "multiple DeleteWhen one true marks for deletion",
			opts: []ResourceOption{DeleteWhen(false), DeleteWhen(true), DeleteWhen(false)},
			want: resourceOptions{Delete: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "enabled gate with one DeleteWhen true marks for deletion",
			opts: []ResourceOption{GatedBy(&enabledFeature{}), DeleteWhen(true)},
			want: resourceOptions{Delete: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "disabled gate ignores false DeleteWhen",
			opts: []ResourceOption{GatedBy(&disabledFeature{}), DeleteWhen(false)},
			want: resourceOptions{Delete: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "Auxiliary sets participation mode",
			opts: []ResourceOption{Auxiliary()},
			want: resourceOptions{ParticipationMode: ParticipationModeAuxiliary},
		},
		{
			name: "Auxiliary preserved when deleted",
			opts: []ResourceOption{GatedBy(&disabledFeature{}), Auxiliary()},
			want: resourceOptions{Delete: true, ParticipationMode: ParticipationModeAuxiliary},
		},
		{
			name: "BlockOnForeignController sets flag",
			opts: []ResourceOption{BlockOnForeignController()},
			want: resourceOptions{BlockOnForeignController: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "BlockOnForeignController alongside Unowned",
			opts: []ResourceOption{Unowned(), BlockOnForeignController()},
			want: resourceOptions{Unowned: true, BlockOnForeignController: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "ReadOnly sets flag",
			opts: []ResourceOption{ReadOnly()},
			want: resourceOptions{ReadOnly: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "SuppressGraceInconsistencyWarning sets flag",
			opts: []ResourceOption{SuppressGraceInconsistencyWarning()},
			want: resourceOptions{SuppressGraceInconsistencyWarning: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "BlockOnAbsence alongside ReadOnly",
			opts: []ResourceOption{ReadOnly(), BlockOnAbsence()},
			want: resourceOptions{ReadOnly: true, BlockOnAbsence: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "IgnoreIfAbsent alongside ReadOnly",
			opts: []ResourceOption{ReadOnly(), IgnoreIfAbsent()},
			want: resourceOptions{ReadOnly: true, IgnoreIfAbsent: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "last GatedBy wins",
			opts: []ResourceOption{GatedBy(&disabledFeature{}), GatedBy(&enabledFeature{})},
			want: resourceOptions{ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "Unowned sets unowned flag",
			opts: []ResourceOption{Unowned()},
			want: resourceOptions{Unowned: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "Unowned combined with Auxiliary",
			opts: []ResourceOption{Unowned(), Auxiliary()},
			want: resourceOptions{Unowned: true, ParticipationMode: ParticipationModeAuxiliary},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveResourceOptions(tt.opts)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolve_OrphanWhen(t *testing.T) {
	t.Run("orphan when false is managed normally", func(t *testing.T) {
		opts, err := resolveResourceOptions([]ResourceOption{OrphanWhen(false)})
		require.NoError(t, err)
		assert.False(t, opts.Orphan)
		assert.False(t, opts.Delete)
	})
	t.Run("orphan when true sets Orphan", func(t *testing.T) {
		opts, err := resolveResourceOptions([]ResourceOption{OrphanWhen(true)})
		require.NoError(t, err)
		assert.True(t, opts.Orphan)
		assert.False(t, opts.Delete)
	})
	t.Run("orphan conditions are additive (any true)", func(t *testing.T) {
		opts, err := resolveResourceOptions([]ResourceOption{OrphanWhen(false), OrphanWhen(true)})
		require.NoError(t, err)
		assert.True(t, opts.Orphan)
	})
}

func TestResolve_OrphanWhen_Exclusivity(t *testing.T) {
	cases := []struct {
		name string
		opts []ResourceOption
		want string
	}{
		{"orphan + delete", []ResourceOption{OrphanWhen(true), Delete()}, "OrphanWhen is mutually exclusive with Delete"},
		{"orphan + deleteWhen", []ResourceOption{OrphanWhen(true), DeleteWhen(true)}, "OrphanWhen is mutually exclusive with Delete"},
		{"orphan + gatedBy", []ResourceOption{OrphanWhen(true), GatedBy(feature.NewBooleanGate(true))}, "OrphanWhen is mutually exclusive with GatedBy"},
		{"orphan + readonly", []ResourceOption{OrphanWhen(true), ReadOnly()}, "OrphanWhen is mutually exclusive with ReadOnly"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveResourceOptions(tc.opts)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestResolveResourceOptions_ValidationErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		opts      []ResourceOption
		wantErrIs string
	}{
		{
			name:      "IgnoreIfAbsent without ReadOnly errors",
			opts:      []ResourceOption{IgnoreIfAbsent()},
			wantErrIs: "IgnoreIfAbsent requires ReadOnly",
		},
		{
			name:      "BlockOnForeignController with ReadOnly errors",
			opts:      []ResourceOption{ReadOnly(), BlockOnForeignController()},
			wantErrIs: "BlockOnForeignController is mutually exclusive with ReadOnly",
		},
		{
			name:      "BlockOnAbsence without ReadOnly errors",
			opts:      []ResourceOption{BlockOnAbsence()},
			wantErrIs: "BlockOnAbsence requires ReadOnly",
		},
		{
			name:      "BlockOnAbsence and IgnoreIfAbsent mutually exclusive",
			opts:      []ResourceOption{ReadOnly(), BlockOnAbsence(), IgnoreIfAbsent()},
			wantErrIs: "BlockOnAbsence and IgnoreIfAbsent are mutually exclusive",
		},
		{
			name:      "ReadOnly with Delete errors",
			opts:      []ResourceOption{ReadOnly(), Delete()},
			wantErrIs: "ReadOnly is mutually exclusive with Delete and DeleteWhen",
		},
		{
			name:      "ReadOnly with DeleteWhen(true) errors",
			opts:      []ResourceOption{ReadOnly(), DeleteWhen(true)},
			wantErrIs: "ReadOnly is mutually exclusive with Delete and DeleteWhen",
		},
		{
			name:      "ReadOnly with DeleteWhen(false) errors on presence, not value",
			opts:      []ResourceOption{ReadOnly(), DeleteWhen(false)},
			wantErrIs: "ReadOnly is mutually exclusive with Delete and DeleteWhen",
		},
		{
			name:      "ReadOnly with disabled GatedBy errors",
			opts:      []ResourceOption{ReadOnly(), GatedBy(&disabledFeature{})},
			wantErrIs: "ReadOnly is mutually exclusive with GatedBy",
		},
		{
			name:      "ReadOnly with enabled GatedBy errors on presence, not value",
			opts:      []ResourceOption{ReadOnly(), GatedBy(&enabledFeature{})},
			wantErrIs: "ReadOnly is mutually exclusive with GatedBy",
		},
		{
			name:      "ReadOnly with BlockOnAbsence and disabled gate now errors on gate",
			opts:      []ResourceOption{GatedBy(&disabledFeature{}), ReadOnly(), BlockOnAbsence()},
			wantErrIs: "ReadOnly is mutually exclusive with GatedBy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveResourceOptions(tt.opts)
			if tt.wantErrIs == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrIs)
		})
	}
}
