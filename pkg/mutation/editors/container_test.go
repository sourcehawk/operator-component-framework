package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestContainerEditor_Raw(t *testing.T) {
	c := &corev1.Container{}
	e := NewContainerEditor(c)
	assert.Equal(t, c, e.Raw())
}

func TestContainerEditor_EnsureEnvVar(t *testing.T) {
	c := &corev1.Container{
		Env: []corev1.EnvVar{
			{Name: "FOO", Value: "BAR"},
			{
				Name: "EXISTING_VAL_FROM",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						Key: "key",
					},
				},
			},
		},
	}
	e := NewContainerEditor(c)

	e.EnsureEnvVar(corev1.EnvVar{Name: "FOO", Value: "NEW"})
	assert.Equal(t, "NEW", c.Env[0].Value)
	assert.Nil(t, c.Env[0].ValueFrom)

	e.EnsureEnvVar(corev1.EnvVar{Name: "EXISTING_VAL_FROM", Value: "LITERAL"})
	assert.Equal(t, "LITERAL", c.Env[1].Value)
	assert.Nil(t, c.Env[1].ValueFrom)

	e.EnsureEnvVar(corev1.EnvVar{Name: "BAR", Value: "VAL"})
	assert.Len(t, c.Env, 3)
	assert.Equal(t, "BAR", c.Env[2].Name)
	assert.Equal(t, "VAL", c.Env[2].Value)
}

func TestContainerEditor_EnsureEnvVar_ValueFrom(t *testing.T) {
	c := &corev1.Container{}
	e := NewContainerEditor(c)

	ev := corev1.EnvVar{
		Name: "SECRET_VAR",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
				Key:                  "my-key",
			},
		},
	}
	e.EnsureEnvVar(ev)
	assert.Len(t, c.Env, 1)
	assert.Equal(t, "SECRET_VAR", c.Env[0].Name)
	assert.Equal(t, "", c.Env[0].Value)
	assert.NotNil(t, c.Env[0].ValueFrom)
	assert.Equal(t, "my-secret", c.Env[0].ValueFrom.SecretKeyRef.Name)
}

func TestContainerEditor_EnsureEnvVars(t *testing.T) {
	c := &corev1.Container{}
	e := NewContainerEditor(c)

	e.EnsureEnvVars([]corev1.EnvVar{
		{Name: "MULTI1", Value: "VAL1"},
		{Name: "MULTI2", Value: "VAL2"},
	})
	assert.Len(t, c.Env, 2)
	assert.Equal(t, "MULTI1", c.Env[0].Name)
	assert.Equal(t, "VAL1", c.Env[0].Value)
	assert.Equal(t, "MULTI2", c.Env[1].Name)
	assert.Equal(t, "VAL2", c.Env[1].Value)
}

func TestContainerEditor_RemoveEnvVar(t *testing.T) {
	c := &corev1.Container{
		Env: []corev1.EnvVar{
			{Name: "FOO", Value: "BAR"},
			{Name: "BAZ", Value: "QUX"},
		},
	}
	e := NewContainerEditor(c)

	e.RemoveEnvVar("FOO")
	assert.Len(t, c.Env, 1)
	assert.Equal(t, "BAZ", c.Env[0].Name)
}

func TestContainerEditor_RemoveEnvVars(t *testing.T) {
	c := &corev1.Container{
		Env: []corev1.EnvVar{
			{Name: "FOO", Value: "BAR"},
			{Name: "BAZ", Value: "QUX"},
			{Name: "KEEP", Value: "STAY"},
		},
	}
	e := NewContainerEditor(c)

	e.RemoveEnvVars([]string{"FOO", "BAZ"})
	assert.Len(t, c.Env, 1)
	assert.Equal(t, "KEEP", c.Env[0].Name)
}

func TestContainerEditor_EnsureArg(t *testing.T) {
	c := &corev1.Container{
		Args: []string{"--arg1"},
	}
	e := NewContainerEditor(c)

	e.EnsureArg("--arg2")
	assert.Contains(t, c.Args, "--arg2")
	e.EnsureArg("--arg1")
	assert.Len(t, c.Args, 2)
}

func TestContainerEditor_EnsureArgs(t *testing.T) {
	c := &corev1.Container{}
	e := NewContainerEditor(c)

	e.EnsureArgs([]string{"--arg3", "--arg4"})
	assert.Len(t, c.Args, 2)
	assert.Contains(t, c.Args, "--arg3")
	assert.Contains(t, c.Args, "--arg4")
}

func TestContainerEditor_RemoveArg(t *testing.T) {
	c := &corev1.Container{
		Args: []string{"--arg1", "--arg2"},
	}
	e := NewContainerEditor(c)

	e.RemoveArg("--arg1")
	assert.Len(t, c.Args, 1)
	assert.Equal(t, "--arg2", c.Args[0])
}

func TestContainerEditor_RemoveArgs(t *testing.T) {
	c := &corev1.Container{
		Args: []string{"--arg1", "--arg2", "--arg3"},
	}
	e := NewContainerEditor(c)

	e.RemoveArgs([]string{"--arg1", "--arg3"})
	assert.Len(t, c.Args, 1)
	assert.Equal(t, "--arg2", c.Args[0])
}

func TestContainerEditor_SetResourceLimit(t *testing.T) {
	c := &corev1.Container{}
	e := NewContainerEditor(c)

	q := resource.MustParse("100m")
	e.SetResourceLimit(corev1.ResourceCPU, q)

	assert.Equal(t, q, c.Resources.Limits[corev1.ResourceCPU])
}

func TestContainerEditor_SetResourceRequest(t *testing.T) {
	c := &corev1.Container{}
	e := NewContainerEditor(c)

	q := resource.MustParse("128Mi")
	e.SetResourceRequest(corev1.ResourceMemory, q)

	assert.Equal(t, q, c.Resources.Requests[corev1.ResourceMemory])
}

func TestContainerEditor_SetResources(t *testing.T) {
	c := &corev1.Container{}
	e := NewContainerEditor(c)

	res := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU: resource.MustParse("100m"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	}
	e.SetResources(res)

	assert.Equal(t, res, c.Resources)
}
