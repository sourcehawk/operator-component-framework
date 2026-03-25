package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
)

func TestIngressSpecEditor(t *testing.T) {
	t.Run("Raw", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{}
		editor := NewIngressSpecEditor(spec)
		assert.Equal(t, spec, editor.Raw())
	})

	t.Run("SetIngressClassName", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{}
		editor := NewIngressSpecEditor(spec)
		editor.SetIngressClassName("nginx")
		require.NotNil(t, spec.IngressClassName)
		assert.Equal(t, "nginx", *spec.IngressClassName)
	})

	t.Run("SetDefaultBackend", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{}
		editor := NewIngressSpecEditor(spec)
		backend := &networkingv1.IngressBackend{
			Service: &networkingv1.IngressServiceBackend{
				Name: "my-svc",
				Port: networkingv1.ServiceBackendPort{Number: 80},
			},
		}
		editor.SetDefaultBackend(backend)
		assert.Equal(t, backend, spec.DefaultBackend)
	})

	t.Run("SetDefaultBackend nil", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{
			DefaultBackend: &networkingv1.IngressBackend{},
		}
		editor := NewIngressSpecEditor(spec)
		editor.SetDefaultBackend(nil)
		assert.Nil(t, spec.DefaultBackend)
	})
}

func TestIngressSpecEditor_EnsureRule(t *testing.T) {
	t.Run("appends new rule", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{}
		editor := NewIngressSpecEditor(spec)
		rule := networkingv1.IngressRule{Host: "example.com"}
		editor.EnsureRule(rule)
		require.Len(t, spec.Rules, 1)
		assert.Equal(t, "example.com", spec.Rules[0].Host)
	})

	t.Run("upserts existing rule by host", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "example.com"},
				{Host: "other.com"},
			},
		}
		editor := NewIngressSpecEditor(spec)
		updated := networkingv1.IngressRule{
			Host: "example.com",
			IngressRuleValue: networkingv1.IngressRuleValue{
				HTTP: &networkingv1.HTTPIngressRuleValue{
					Paths: []networkingv1.HTTPIngressPath{
						{Path: "/new"},
					},
				},
			},
		}
		editor.EnsureRule(updated)
		require.Len(t, spec.Rules, 2)
		require.NotNil(t, spec.Rules[0].HTTP)
		assert.Equal(t, "/new", spec.Rules[0].HTTP.Paths[0].Path)
		assert.Equal(t, "other.com", spec.Rules[1].Host)
	})
}

func TestIngressSpecEditor_RemoveRule(t *testing.T) {
	t.Run("removes existing rule", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "example.com"},
				{Host: "other.com"},
			},
		}
		editor := NewIngressSpecEditor(spec)
		editor.RemoveRule("example.com")
		require.Len(t, spec.Rules, 1)
		assert.Equal(t, "other.com", spec.Rules[0].Host)
	})

	t.Run("no-op for missing host", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{Host: "example.com"},
			},
		}
		editor := NewIngressSpecEditor(spec)
		editor.RemoveRule("nonexistent.com")
		require.Len(t, spec.Rules, 1)
	})
}

func TestIngressSpecEditor_EnsureTLS(t *testing.T) {
	t.Run("appends new TLS entry", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{}
		editor := NewIngressSpecEditor(spec)
		tls := networkingv1.IngressTLS{
			Hosts:      []string{"example.com"},
			SecretName: "tls-secret",
		}
		editor.EnsureTLS(tls)
		require.Len(t, spec.TLS, 1)
		assert.Equal(t, "tls-secret", spec.TLS[0].SecretName)
	})

	t.Run("upserts existing TLS by first host", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{
				{Hosts: []string{"example.com"}, SecretName: "old-secret"},
				{Hosts: []string{"other.com"}, SecretName: "other-secret"},
			},
		}
		editor := NewIngressSpecEditor(spec)
		tls := networkingv1.IngressTLS{
			Hosts:      []string{"example.com"},
			SecretName: "new-secret",
		}
		editor.EnsureTLS(tls)
		require.Len(t, spec.TLS, 2)
		assert.Equal(t, "new-secret", spec.TLS[0].SecretName)
		assert.Equal(t, "other-secret", spec.TLS[1].SecretName)
	})

	t.Run("ignores TLS with empty hosts", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{}
		editor := NewIngressSpecEditor(spec)
		tls := networkingv1.IngressTLS{SecretName: "orphan"}
		editor.EnsureTLS(tls)
		assert.Empty(t, spec.TLS)
	})
}

func TestIngressSpecEditor_RemoveTLS(t *testing.T) {
	t.Run("removes matching TLS entries", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{
				{Hosts: []string{"example.com"}, SecretName: "s1"},
				{Hosts: []string{"other.com"}, SecretName: "s2"},
				{Hosts: []string{"third.com"}, SecretName: "s3"},
			},
		}
		editor := NewIngressSpecEditor(spec)
		editor.RemoveTLS("example.com", "third.com")
		require.Len(t, spec.TLS, 1)
		assert.Equal(t, "s2", spec.TLS[0].SecretName)
	})

	t.Run("no-op for missing hosts", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{
				{Hosts: []string{"example.com"}, SecretName: "s1"},
			},
		}
		editor := NewIngressSpecEditor(spec)
		editor.RemoveTLS("nonexistent.com")
		require.Len(t, spec.TLS, 1)
	})

	t.Run("handles TLS entries with empty hosts", func(t *testing.T) {
		spec := &networkingv1.IngressSpec{
			TLS: []networkingv1.IngressTLS{
				{Hosts: []string{"example.com"}, SecretName: "s1"},
				{SecretName: "no-hosts"},
			},
		}
		editor := NewIngressSpecEditor(spec)
		editor.RemoveTLS("example.com")
		require.Len(t, spec.TLS, 1)
		assert.Equal(t, "no-hosts", spec.TLS[0].SecretName)
	})
}
