package editors

import (
	corev1 "k8s.io/api/core/v1"
)

// ServiceSpecEditor provides a typed API for mutating a Kubernetes ServiceSpec.
type ServiceSpecEditor struct {
	spec *corev1.ServiceSpec
}

// NewServiceSpecEditor creates a new ServiceSpecEditor for the given ServiceSpec.
func NewServiceSpecEditor(spec *corev1.ServiceSpec) *ServiceSpecEditor {
	return &ServiceSpecEditor{spec: spec}
}

// Raw returns the underlying *corev1.ServiceSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *ServiceSpecEditor) Raw() *corev1.ServiceSpec {
	return e.spec
}

// SetType sets the service type (ClusterIP, NodePort, LoadBalancer, ExternalName).
func (e *ServiceSpecEditor) SetType(t corev1.ServiceType) {
	e.spec.Type = t
}

// EnsurePort upserts a service port. If a port with the same Name exists (or the same
// Port number when Name is empty), it is replaced; otherwise the port is appended.
func (e *ServiceSpecEditor) EnsurePort(port corev1.ServicePort) {
	for i, existing := range e.spec.Ports {
		if port.Name != "" && existing.Name == port.Name {
			e.spec.Ports[i] = port
			return
		}
		if port.Name == "" && existing.Port == port.Port {
			e.spec.Ports[i] = port
			return
		}
	}
	e.spec.Ports = append(e.spec.Ports, port)
}

// RemovePort removes a service port by name. It is a no-op if the port does not exist.
func (e *ServiceSpecEditor) RemovePort(name string) {
	for i, existing := range e.spec.Ports {
		if existing.Name == name {
			e.spec.Ports = append(e.spec.Ports[:i], e.spec.Ports[i+1:]...)
			return
		}
	}
}

// SetSelector replaces the entire service selector map.
func (e *ServiceSpecEditor) SetSelector(selector map[string]string) {
	e.spec.Selector = selector
}

// EnsureSelector adds or updates a single selector key-value pair.
func (e *ServiceSpecEditor) EnsureSelector(key, value string) {
	if e.spec.Selector == nil {
		e.spec.Selector = make(map[string]string)
	}
	e.spec.Selector[key] = value
}

// RemoveSelector removes a single selector key. It is a no-op if the key does not exist.
func (e *ServiceSpecEditor) RemoveSelector(key string) {
	if e.spec.Selector != nil {
		delete(e.spec.Selector, key)
	}
}

// SetSessionAffinity sets the session affinity type (None or ClientIP).
func (e *ServiceSpecEditor) SetSessionAffinity(affinity corev1.ServiceAffinity) {
	e.spec.SessionAffinity = affinity
}

// SetSessionAffinityConfig sets the session affinity configuration.
func (e *ServiceSpecEditor) SetSessionAffinityConfig(cfg *corev1.SessionAffinityConfig) {
	e.spec.SessionAffinityConfig = cfg
}

// SetPublishNotReadyAddresses controls whether endpoints for not-ready pods are published.
func (e *ServiceSpecEditor) SetPublishNotReadyAddresses(v bool) {
	e.spec.PublishNotReadyAddresses = v
}

// SetExternalTrafficPolicy sets the external traffic policy for the service.
func (e *ServiceSpecEditor) SetExternalTrafficPolicy(policy corev1.ServiceExternalTrafficPolicy) {
	e.spec.ExternalTrafficPolicy = policy
}

// SetInternalTrafficPolicy sets the internal traffic policy for the service.
func (e *ServiceSpecEditor) SetInternalTrafficPolicy(policy *corev1.ServiceInternalTrafficPolicy) {
	e.spec.InternalTrafficPolicy = policy
}

// SetLoadBalancerSourceRanges sets the allowed source ranges for a LoadBalancer service.
func (e *ServiceSpecEditor) SetLoadBalancerSourceRanges(ranges []string) {
	e.spec.LoadBalancerSourceRanges = ranges
}

// SetExternalName sets the external name for an ExternalName service type.
func (e *ServiceSpecEditor) SetExternalName(name string) {
	e.spec.ExternalName = name
}
