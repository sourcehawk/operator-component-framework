package scaffold

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validOptions() Options {
	return Options{
		Type:     "k8s.io/api/apps/v1.Deployment",
		Variant:  "workload",
		Group:    "apps",
		GroupSet: true,
	}
}

func TestResolveDerivesDefaults(t *testing.T) {
	t.Parallel()

	data, err := validOptions().Resolve()
	require.NoError(t, err)

	assert.Equal(t, "k8s.io/api/apps/v1", data.ImportPath)
	assert.Equal(t, "appsv1", data.ImportAlias)
	assert.Equal(t, "Deployment", data.TypeName)
	assert.Equal(t, "Deployment", data.Kind)
	assert.Equal(t, "v1", data.Version)
	assert.Equal(t, "apps", data.Group)
	assert.Equal(t, "deployment", data.Package)
	assert.Equal(t, VariantWorkload, data.Variant)
	assert.False(t, data.ClusterScoped)
}

func TestResolveDerivations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		mutate        func(*Options)
		expectedAlias string
		expectedVer   string
		expectedPkg   string
		expectedKind  string
	}{
		{
			name:          "core group type",
			mutate:        func(o *Options) { o.Type = "k8s.io/api/core/v1.ConfigMap"; o.Group = ""; o.Variant = "static" },
			expectedAlias: "corev1",
			expectedVer:   "v1",
			expectedPkg:   "configmap",
			expectedKind:  "ConfigMap",
		},
		{
			name: "third party crd with dashed segment",
			mutate: func(o *Options) {
				o.Type = "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1.Certificate"
				o.Group = "cert-manager.io"
			},
			expectedAlias: "certmanagerv1",
			expectedVer:   "v1",
			expectedPkg:   "certificate",
			expectedKind:  "Certificate",
		},
		{
			name: "beta version segment",
			mutate: func(o *Options) {
				o.Type = "example.io/api/messaging/v1beta2.Queue"
				o.Group = "messaging.example.io"
			},
			expectedAlias: "messagingv1beta2",
			expectedVer:   "v1beta2",
			expectedPkg:   "queue",
			expectedKind:  "Queue",
		},
		{
			name: "multi segment import path with dashes and underscores",
			mutate: func(o *Options) {
				o.Type = "example.io/go-api/v2_x/messaging/v1beta2.Queue"
				o.Group = "messaging.example.io"
			},
			expectedAlias: "messagingv1beta2",
			expectedVer:   "v1beta2",
			expectedPkg:   "queue",
			expectedKind:  "Queue",
		},
		{
			name: "explicit overrides win",
			mutate: func(o *Options) {
				o.Alias = "customalias"
				o.Version = "v2"
				o.Package = "mypkg"
				o.Kind = "OtherKind"
			},
			expectedAlias: "customalias",
			expectedVer:   "v2",
			expectedPkg:   "mypkg",
			expectedKind:  "OtherKind",
		},
		{
			name: "non version last segment derives alias from that segment",
			mutate: func(o *Options) {
				o.Type = "example.io/apis/messaging.Queue"
				o.Version = "v1"
				o.Group = "messaging.example.io"
			},
			expectedAlias: "messaging",
			expectedVer:   "v1",
			expectedPkg:   "queue",
			expectedKind:  "Queue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := validOptions()
			tt.mutate(&opts)

			data, err := opts.Resolve()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedAlias, data.ImportAlias)
			assert.Equal(t, tt.expectedVer, data.Version)
			assert.Equal(t, tt.expectedPkg, data.Package)
			assert.Equal(t, tt.expectedKind, data.Kind)
		})
	}
}

func TestResolveAcceptsValidGroupsAndVersions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		mutate          func(*Options)
		expectedGroup   string
		expectedVersion string
	}{
		{
			name:            "non core group",
			mutate:          func(o *Options) { o.Group = "rbac.authorization.k8s.io" },
			expectedGroup:   "rbac.authorization.k8s.io",
			expectedVersion: "v1",
		},
		{
			name:            "dashed group label",
			mutate:          func(o *Options) { o.Group = "cert-manager.io" },
			expectedGroup:   "cert-manager.io",
			expectedVersion: "v1",
		},
		{
			name:            "empty group is the core API group",
			mutate:          func(o *Options) { o.Group = "" },
			expectedGroup:   "",
			expectedVersion: "v1",
		},
		{
			name:            "explicit version",
			mutate:          func(o *Options) { o.Version = "v2beta1" },
			expectedGroup:   "apps",
			expectedVersion: "v2beta1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := validOptions()
			tt.mutate(&opts)

			data, err := opts.Resolve()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedGroup, data.Group)
			assert.Equal(t, tt.expectedVersion, data.Version)
		})
	}
}

func TestResolveValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(*Options)
		expectedErr string
	}{
		{
			name:        "missing type",
			mutate:      func(o *Options) { o.Type = "" },
			expectedErr: "--type is required",
		},
		{
			name:        "type without dot",
			mutate:      func(o *Options) { o.Type = "k8s.io/api/apps/v1" },
			expectedErr: `--type must be <import-path>.<TypeName>`,
		},
		{
			name:        "unexported type name",
			mutate:      func(o *Options) { o.Type = "k8s.io/api/apps/v1.deployment" },
			expectedErr: `type name "deployment" must be an exported Go identifier`,
		},
		{
			name:        "empty import path",
			mutate:      func(o *Options) { o.Type = ".Deployment" },
			expectedErr: "--type is missing an import path",
		},
		{
			name:   "import path with a Go import break out",
			mutate: func(o *Options) { o.Type = "k8s.io/api/core/v1\"\n\t_ \"os.ConfigMap" },
			expectedErr: "--type import path \"k8s.io/api/core/v1\\\"\\n\\t_ \\\"os\" " +
				"is not a valid Go import path",
		},
		{
			name:        "import path with a space",
			mutate:      func(o *Options) { o.Type = "k8s.io/api/core v1.ConfigMap" },
			expectedErr: `--type import path "k8s.io/api/core v1" is not a valid Go import path`,
		},
		{
			name:        "import path with an empty element",
			mutate:      func(o *Options) { o.Type = "example.io//v1.Queue" },
			expectedErr: `--type import path "example.io//v1" is not a valid Go import path`,
		},
		{
			name:        "missing variant",
			mutate:      func(o *Options) { o.Variant = "" },
			expectedErr: "--variant is required",
		},
		{
			name:        "unknown variant",
			mutate:      func(o *Options) { o.Variant = "daemon" },
			expectedErr: `--variant must be one of static, workload, task, integration; got "daemon"`,
		},
		{
			name:        "group not provided",
			mutate:      func(o *Options) { o.Group = ""; o.GroupSet = false },
			expectedErr: `--group is required (pass --group "" for core API group types)`,
		},
		{
			name:        "version not derivable",
			mutate:      func(o *Options) { o.Type = "example.io/apis/messaging.Queue" },
			expectedErr: `--version is required: the last segment "messaging" of the import path is not an API version`,
		},
		{
			name:        "uppercase group",
			mutate:      func(o *Options) { o.Group = "Apps" },
			expectedErr: `--group "Apps" is not a valid API group (a DNS subdomain, or "" for core API group types)`,
		},
		{
			name:        "group label ends with a dash",
			mutate:      func(o *Options) { o.Group = "apps-.io" },
			expectedErr: `--group "apps-.io" is not a valid API group`,
		},
		{
			name:        "group with a Go string break out",
			mutate:      func(o *Options) { o.Group = `a"+os.Getenv("X")+"b` },
			expectedErr: `--group "a\"+os.Getenv(\"X\")+\"b" is not a valid API group`,
		},
		{
			name:        "explicit version is not an API version",
			mutate:      func(o *Options) { o.Version = "1.0" },
			expectedErr: `--version "1.0" is not a valid API version`,
		},
		{
			name:        "invalid package name",
			mutate:      func(o *Options) { o.Package = "My-Package" },
			expectedErr: `--package "My-Package" is not a valid Go package name`,
		},
		{
			name:        "reserved package name",
			mutate:      func(o *Options) { o.Package = "func" },
			expectedErr: `--package "func" is a Go keyword`,
		},
		{
			name:        "invalid alias",
			mutate:      func(o *Options) { o.Alias = "apps/v1" },
			expectedErr: `--alias "apps/v1" is not a valid Go identifier`,
		},
		{
			name:        "invalid kind",
			mutate:      func(o *Options) { o.Kind = "my kind" },
			expectedErr: `--kind "my kind" must be an exported Go identifier`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := validOptions()
			tt.mutate(&opts)

			_, err := opts.Resolve()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestTemplateDataIdentity(t *testing.T) {
	t.Parallel()

	namespaced := TemplateData{Group: "apps", Version: "v1", Kind: "Deployment"}
	assert.Equal(t, "apps/v1", namespaced.APIVersion())
	assert.Equal(t, "apps/v1/Deployment/%s/%s", namespaced.IdentityFormat())
	assert.Equal(t, "o.Namespace, o.Name", namespaced.IdentityArgs())

	core := TemplateData{Group: "", Version: "v1", Kind: "ConfigMap"}
	assert.Equal(t, "v1", core.APIVersion())
	assert.Equal(t, "v1/ConfigMap/%s/%s", core.IdentityFormat())

	clusterScoped := TemplateData{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole", ClusterScoped: true}
	assert.Equal(t, "rbac.authorization.k8s.io/v1/ClusterRole/%s", clusterScoped.IdentityFormat())
	assert.Equal(t, "o.Name", clusterScoped.IdentityArgs())
}

func TestTemplateDataTypeNames(t *testing.T) {
	t.Parallel()

	data := TemplateData{ImportAlias: "appsv1", TypeName: "Deployment"}
	assert.Equal(t, "appsv1.Deployment", data.QualifiedType())
	assert.Equal(t, "*appsv1.Deployment", data.PointerType())
}

func TestVariantSpecs(t *testing.T) {
	t.Parallel()

	static := VariantStatic.Spec()
	assert.Equal(t, "StaticBuilder", static.GenericBuilder)
	assert.False(t, static.HasStatus)
	assert.False(t, static.HasGrace)
	assert.False(t, static.HasSuspension)
	assert.Empty(t, static.LifecycleInterfaces)

	workload := VariantWorkload.Spec()
	assert.Equal(t, "NewWorkloadBuilder", workload.GenericConstructor)
	assert.Equal(t, "WithCustomConvergeStatus", workload.StatusSetter)
	assert.Equal(t, "concepts.AliveStatusWithReason", workload.StatusResult)
	assert.Equal(t, "concepts.AliveConvergingStatusHealthy", workload.StatusConstant)
	assert.True(t, workload.HasGrace)
	assert.True(t, workload.HasSuspension)
	assert.Equal(t, []string{
		"concepts.Alive: for health and readiness tracking.",
		"concepts.Graceful: for health reporting once the grace period expires.",
	}, workload.LifecycleInterfaces)

	task := VariantTask.Spec()
	assert.Equal(t, "concepts.CompletionStatusWithReason", task.StatusResult)
	assert.Equal(t, "concepts.CompletionStatusCompleted", task.StatusConstant)
	assert.False(t, task.HasGrace)
	assert.True(t, task.HasSuspension)
	assert.Equal(t, []string{"concepts.Completable: for run-to-completion tracking."}, task.LifecycleInterfaces)

	integration := VariantIntegration.Spec()
	assert.Equal(t, "WithCustomOperationalStatus", integration.StatusSetter)
	assert.Equal(t, "DefaultOperationalStatusHandler", integration.StatusHandler)
	assert.Equal(t, "concepts.OperationalStatusOperational", integration.StatusConstant)
	assert.True(t, integration.HasGrace)
	assert.Equal(t, []string{
		"concepts.Operational: for external-dependency readiness tracking.",
		"concepts.Graceful: for health reporting once the grace period expires.",
	}, integration.LifecycleInterfaces)
}
