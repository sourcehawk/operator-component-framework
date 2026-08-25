package observability_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sourcehawk/operator-component-framework/internal/observability"
)

func validOptions() observability.Options {
	return observability.Options{
		MetricNamespace: "demo",
		NamespaceLabel:  observability.DefaultNamespaceLabel,
		AlertFormat:     observability.AlertFormatPrometheusRule,
	}
}

func TestOptionsValidateAccepts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*observability.Options)
	}{
		{name: "defaults", mutate: func(*observability.Options) {}},
		{name: "rules format", mutate: func(o *observability.Options) { o.AlertFormat = observability.AlertFormatRules }},
		{name: "namespace label", mutate: func(o *observability.Options) { o.NamespaceLabel = "namespace" }},
		{name: "underscore and digits", mutate: func(o *observability.Options) { o.MetricNamespace = "My_Op2" }},
		{name: "17 characters", mutate: func(o *observability.Options) { o.MetricNamespace = strings.Repeat("a", 17) }},
		{name: "labels", mutate: func(o *observability.Options) { o.PrometheusRuleLabels = map[string]string{"release": "kps"} }},
		{name: "empty label value", mutate: func(o *observability.Options) { o.PrometheusRuleLabels = map[string]string{"k": ""} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := validOptions()
			tt.mutate(&opts)
			assert.NoError(t, opts.Validate())
		})
	}
}

func TestOptionsValidateRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(*observability.Options)
		expectedErr string
	}{
		{
			name:        "missing metric namespace",
			mutate:      func(o *observability.Options) { o.MetricNamespace = "" },
			expectedErr: "--metric-namespace is required",
		},
		{
			name:        "hyphen in metric namespace",
			mutate:      func(o *observability.Options) { o.MetricNamespace = "my-operator" },
			expectedErr: `--metric-namespace "my-operator" is not renderable`,
		},
		{
			name:        "colon in metric namespace",
			mutate:      func(o *observability.Options) { o.MetricNamespace = "my:op" },
			expectedErr: `--metric-namespace "my:op" is not renderable`,
		},
		{
			name:        "metric namespace starting with a digit",
			mutate:      func(o *observability.Options) { o.MetricNamespace = "1op" },
			expectedErr: "must start with a letter",
		},
		{
			name:        "18 character metric namespace",
			mutate:      func(o *observability.Options) { o.MetricNamespace = strings.Repeat("a", 18) },
			expectedErr: "at most 17 characters",
		},
		{
			name:        "empty namespace label",
			mutate:      func(o *observability.Options) { o.NamespaceLabel = "" },
			expectedErr: `--namespace-label "" is not a Prometheus label name`,
		},
		{
			name:        "namespace label starting with a digit",
			mutate:      func(o *observability.Options) { o.NamespaceLabel = "1ns" },
			expectedErr: `--namespace-label "1ns" is not a Prometheus label name`,
		},
		{
			name:        "namespace label with a hyphen",
			mutate:      func(o *observability.Options) { o.NamespaceLabel = "my-ns" },
			expectedErr: "--namespace-label",
		},
		{
			name:        "unknown alert format",
			mutate:      func(o *observability.Options) { o.AlertFormat = "yaml" },
			expectedErr: `--alert-format must be prometheusrule or rules, got "yaml"`,
		},
		{
			name:        "empty alert format",
			mutate:      func(o *observability.Options) { o.AlertFormat = "" },
			expectedErr: "--alert-format must be prometheusrule or rules",
		},
		{
			name:        "empty label key",
			mutate:      func(o *observability.Options) { o.PrometheusRuleLabels = map[string]string{"": "v"} },
			expectedErr: "--prometheusrule-labels contains an entry with an empty key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := validOptions()
			tt.mutate(&opts)
			err := opts.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestParseLabels(t *testing.T) {
	t.Parallel()

	labels, err := observability.ParseLabels("")
	require.NoError(t, err)
	assert.Nil(t, labels)

	labels, err = observability.ParseLabels("release=kps,team=platform,empty=")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"release": "kps", "team": "platform", "empty": ""}, labels)

	labels, err = observability.ParseLabels("url=a=b")
	require.NoError(t, err, "only the first = splits key from value")
	assert.Equal(t, map[string]string{"url": "a=b"}, labels)

	_, err = observability.ParseLabels("release")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `--prometheusrule-labels entry "release" is not key=value`)

	_, err = observability.ParseLabels("release=kps,=v")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `entry "=v" is not key=value`)
}
