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

func TestResolveResourceOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    []ResourceOption
		want    resourceOptions
		wantErr bool
	}{
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
			name: "ReadOnly sets flag",
			opts: []ResourceOption{ReadOnly()},
			want: resourceOptions{ReadOnly: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "ReadOnly forced false when deleted",
			opts: []ResourceOption{GatedBy(&disabledFeature{}), ReadOnly()},
			want: resourceOptions{Delete: true, ReadOnly: false, ParticipationMode: ParticipationModeRequired},
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
			name: "BlockOnAbsence preserved when deletion forced",
			opts: []ResourceOption{GatedBy(&disabledFeature{}), ReadOnly(), BlockOnAbsence()},
			want: resourceOptions{Delete: true, ReadOnly: false, BlockOnAbsence: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "IgnoreIfAbsent alongside ReadOnly",
			opts: []ResourceOption{ReadOnly(), IgnoreIfAbsent()},
			want: resourceOptions{ReadOnly: true, IgnoreIfAbsent: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "IgnoreIfAbsent preserved when deletion forced",
			opts: []ResourceOption{GatedBy(&disabledFeature{}), ReadOnly(), IgnoreIfAbsent()},
			want: resourceOptions{Delete: true, ReadOnly: false, IgnoreIfAbsent: true, ParticipationMode: ParticipationModeRequired},
		},
		{
			name: "last GatedBy wins",
			opts: []ResourceOption{GatedBy(&disabledFeature{}), GatedBy(&enabledFeature{})},
			want: resourceOptions{ParticipationMode: ParticipationModeRequired},
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

func TestResolveResourceOptions_ValidationErrors(t *testing.T) {
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
			name:      "BlockOnAbsence + ReadOnly + disabled gate does not error",
			opts:      []ResourceOption{GatedBy(&disabledFeature{}), ReadOnly(), BlockOnAbsence()},
			wantErrIs: "",
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
