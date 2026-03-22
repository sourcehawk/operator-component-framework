package editors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServiceSpecEditor(t *testing.T) {
	t.Run("SetType", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		editor.SetType(corev1.ServiceTypeLoadBalancer)
		assert.Equal(t, corev1.ServiceTypeLoadBalancer, spec.Type)
	})

	t.Run("EnsurePort appends new port", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		editor.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
		assert.Len(t, spec.Ports, 1)
		assert.Equal(t, "http", spec.Ports[0].Name)
		assert.Equal(t, int32(80), spec.Ports[0].Port)
	})

	t.Run("EnsurePort upserts by name", func(t *testing.T) {
		spec := &corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
			},
		}
		editor := NewServiceSpecEditor(spec)
		editor.EnsurePort(corev1.ServicePort{Name: "http", Port: 8080, TargetPort: intstr.FromInt32(8080)})
		assert.Len(t, spec.Ports, 1)
		assert.Equal(t, int32(8080), spec.Ports[0].Port)
	})

	t.Run("EnsurePort upserts by port number when name is empty", func(t *testing.T) {
		spec := &corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Port: 80},
			},
		}
		editor := NewServiceSpecEditor(spec)
		editor.EnsurePort(corev1.ServicePort{Port: 80, TargetPort: intstr.FromInt32(8080)})
		assert.Len(t, spec.Ports, 1)
		assert.Equal(t, intstr.FromInt32(8080), spec.Ports[0].TargetPort)
	})

	t.Run("EnsurePort appends when no match", func(t *testing.T) {
		spec := &corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
			},
		}
		editor := NewServiceSpecEditor(spec)
		editor.EnsurePort(corev1.ServicePort{Name: "https", Port: 443})
		assert.Len(t, spec.Ports, 2)
	})

	t.Run("RemovePort removes existing", func(t *testing.T) {
		spec := &corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
				{Name: "https", Port: 443},
			},
		}
		editor := NewServiceSpecEditor(spec)
		editor.RemovePort("http")
		assert.Len(t, spec.Ports, 1)
		assert.Equal(t, "https", spec.Ports[0].Name)
	})

	t.Run("RemovePort no-op when absent", func(t *testing.T) {
		spec := &corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80},
			},
		}
		editor := NewServiceSpecEditor(spec)
		editor.RemovePort("missing")
		assert.Len(t, spec.Ports, 1)
	})

	t.Run("SetSelector replaces entire selector", func(t *testing.T) {
		spec := &corev1.ServiceSpec{
			Selector: map[string]string{"old": "value"},
		}
		editor := NewServiceSpecEditor(spec)
		editor.SetSelector(map[string]string{"app": "test"})
		assert.Equal(t, map[string]string{"app": "test"}, spec.Selector)
	})

	t.Run("EnsureSelector adds key", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		editor.EnsureSelector("app", "test")
		assert.Equal(t, "test", spec.Selector["app"])
	})

	t.Run("EnsureSelector overwrites key", func(t *testing.T) {
		spec := &corev1.ServiceSpec{
			Selector: map[string]string{"app": "old"},
		}
		editor := NewServiceSpecEditor(spec)
		editor.EnsureSelector("app", "new")
		assert.Equal(t, "new", spec.Selector["app"])
	})

	t.Run("RemoveSelector removes key", func(t *testing.T) {
		spec := &corev1.ServiceSpec{
			Selector: map[string]string{"app": "test", "env": "prod"},
		}
		editor := NewServiceSpecEditor(spec)
		editor.RemoveSelector("app")
		assert.NotContains(t, spec.Selector, "app")
		assert.Equal(t, "prod", spec.Selector["env"])
	})

	t.Run("RemoveSelector no-op on nil selector", func(_ *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		editor.RemoveSelector("missing") // should not panic
	})

	t.Run("SetSessionAffinity", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		editor.SetSessionAffinity(corev1.ServiceAffinityClientIP)
		assert.Equal(t, corev1.ServiceAffinityClientIP, spec.SessionAffinity)
	})

	t.Run("SetSessionAffinityConfig", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		timeout := int32(10800)
		cfg := &corev1.SessionAffinityConfig{
			ClientIP: &corev1.ClientIPConfig{TimeoutSeconds: &timeout},
		}
		editor.SetSessionAffinityConfig(cfg)
		assert.Equal(t, cfg, spec.SessionAffinityConfig)
	})

	t.Run("SetPublishNotReadyAddresses", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		editor.SetPublishNotReadyAddresses(true)
		assert.True(t, spec.PublishNotReadyAddresses)
	})

	t.Run("SetExternalTrafficPolicy", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		editor.SetExternalTrafficPolicy(corev1.ServiceExternalTrafficPolicyLocal)
		assert.Equal(t, corev1.ServiceExternalTrafficPolicyLocal, spec.ExternalTrafficPolicy)
	})

	t.Run("SetInternalTrafficPolicy", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		local := corev1.ServiceInternalTrafficPolicyLocal
		editor.SetInternalTrafficPolicy(&local)
		assert.Equal(t, &local, spec.InternalTrafficPolicy)
	})

	t.Run("SetLoadBalancerSourceRanges", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		ranges := []string{"10.0.0.0/8", "192.168.0.0/16"}
		editor.SetLoadBalancerSourceRanges(ranges)
		assert.Equal(t, ranges, spec.LoadBalancerSourceRanges)
	})

	t.Run("SetExternalName", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		editor.SetExternalName("my.external.service.example.com")
		assert.Equal(t, "my.external.service.example.com", spec.ExternalName)
	})

	t.Run("Raw returns underlying spec", func(t *testing.T) {
		spec := &corev1.ServiceSpec{}
		editor := NewServiceSpecEditor(spec)
		assert.Equal(t, spec, editor.Raw())
	})
}
