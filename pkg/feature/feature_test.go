package feature

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockConstraint struct {
	enabled bool
}

func (m *mockConstraint) Enabled(_ string) (bool, error) {
	return m.enabled, nil
}

type errorConstraint struct{}

func (e *errorConstraint) Enabled(_ string) (bool, error) {
	return false, fmt.Errorf("unexpected error")
}

func TestResourceFeature_Enabled(t *testing.T) {
	// Setup features
	f1 := &mockConstraint{enabled: true}
	f2 := &mockConstraint{enabled: false}

	tests := []struct {
		name              string
		currentVersion    string
		semverConstraints []VersionConstraint
		truths            []bool
		want              bool
		wantErr           bool
	}{
		{
			name:              "no constraints, no truths, enabled (default true)",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{},
			want:              true,
		},
		{
			name:              "version matches single constraint",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{f1},
			want:              true,
		},
		{
			name:              "version does not match single constraint",
			currentVersion:    "8.0.0",
			semverConstraints: []VersionConstraint{f2},
			want:              false,
		},
		{
			name:              "version matches ALL multiple constraints",
			currentVersion:    "8.3.0",
			semverConstraints: []VersionConstraint{f1, f1},
			want:              true,
		},
		{
			name:              "version matches only one of multiple constraints",
			currentVersion:    "8.1.5",
			semverConstraints: []VersionConstraint{f1, f2},
			want:              false,
		},
		{
			name:              "version matches none of multiple constraints",
			currentVersion:    "8.0.0",
			semverConstraints: []VersionConstraint{f2, f2},
			want:              false,
		},
		{
			name:              "truth condition is true",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{f1},
			truths:            []bool{true},
			want:              true,
		},
		{
			name:              "truth condition is false",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{f1},
			truths:            []bool{false},
			want:              false,
		},
		{
			name:              "multiple truth conditions (all true)",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{f1},
			truths:            []bool{true, true},
			want:              true,
		},
		{
			name:              "multiple truth conditions (one false)",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{f1},
			truths:            []bool{true, false},
			want:              false,
		},
		{
			name:              "no version constraints, truth is true",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{},
			truths:            []bool{true},
			want:              true,
		},
		{
			name:              "no version constraints, truth is false",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{},
			truths:            []bool{false},
			want:              false,
		},
		{
			name:              "nil version constraints are ignored",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{f1, nil, f1},
			want:              true,
		},
		{
			name:              "nil version constraints are ignored, remaining one fails",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{nil, f2},
			want:              false,
		},
		{
			name:              "version constraint returns error",
			currentVersion:    "8.1.0",
			semverConstraints: []VersionConstraint{&errorConstraint{}},
			want:              false,
			wantErr:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rf := NewResourceFeature(tt.currentVersion, tt.semverConstraints)
			for _, truth := range tt.truths {
				res := rf.When(truth)
				assert.Same(t, rf, res, "When should return the ResourceFeature pointer for chaining")
			}
			enabled, err := rf.Enabled()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, enabled)
			}
		})
	}
}

func TestResourceFeature_WhenChaining(t *testing.T) {
	rf := NewResourceFeature("8.1.0", nil).
		When(true).
		When(true).
		When(false)

	enabled, err := rf.Enabled()
	assert.NoError(t, err)
	assert.False(t, enabled)
}

func TestMutation_ApplyIntent(t *testing.T) {
	const newValue = "new-value"
	type testObj struct {
		Value string
	}

	fEnabled := &mockConstraint{enabled: true}
	fDisabled := &mockConstraint{enabled: false}

	tests := []struct {
		name      string
		mutation  Mutation[*testObj]
		initial   *testObj
		want      *testObj
		wantErr   bool
		errString string
	}{
		{
			name: "feature enabled, mutation applied",
			mutation: Mutation[*testObj]{
				Name:    "set-value",
				Feature: NewResourceFeature("8.1.0", []VersionConstraint{fEnabled}),
				Mutate: func(obj *testObj) error {
					obj.Value = newValue
					return nil
				},
			},
			initial: &testObj{Value: "old-value"},
			want:    &testObj{Value: newValue},
		},
		{
			name: "feature disabled, mutation not applied",
			mutation: Mutation[*testObj]{
				Name:    "set-value",
				Feature: NewResourceFeature("8.1.0", []VersionConstraint{fDisabled}),
				Mutate: func(obj *testObj) error {
					obj.Value = newValue
					return nil
				},
			},
			initial: &testObj{Value: "old-value"},
			want:    &testObj{Value: "old-value"},
		},
		{
			name: "feature nil, mutation applied unconditionally",
			mutation: Mutation[*testObj]{
				Name:    "set-value",
				Feature: nil,
				Mutate: func(obj *testObj) error {
					obj.Value = newValue
					return nil
				},
			},
			initial: &testObj{Value: "old-value"},
			want:    &testObj{Value: newValue},
		},
		{
			name: "feature nil, mutate nil, returns error",
			mutation: Mutation[*testObj]{
				Name:    "nil-mutate-unconditional",
				Feature: nil,
				Mutate:  nil,
			},
			initial:   &testObj{Value: "old-value"},
			want:      &testObj{Value: "old-value"},
			wantErr:   true,
			errString: "mutation handler of nil-mutate-unconditional is nil",
		},
		{
			name: "feature enabled but mutate is nil, returns error",
			mutation: Mutation[*testObj]{
				Name:    "nil-mutate",
				Feature: NewResourceFeature("8.1.0", []VersionConstraint{fEnabled}),
				Mutate:  nil,
			},
			initial:   &testObj{Value: "old-value"},
			want:      &testObj{Value: "old-value"},
			wantErr:   true,
			errString: "mutation handler of nil-mutate is nil",
		},
		{
			name: "feature constraint returns error, ApplyIntent returns error",
			mutation: Mutation[*testObj]{
				Name:    "error-feature",
				Feature: NewResourceFeature("8.1.0", []VersionConstraint{&errorConstraint{}}),
				Mutate: func(obj *testObj) error {
					obj.Value = newValue
					return nil
				},
			},
			initial:   &testObj{Value: "old-value"},
			want:      &testObj{Value: "old-value"},
			wantErr:   true,
			errString: "unexpected error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.initial
			err := tt.mutation.ApplyIntent(obj)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errString)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, obj)
		})
	}
}
