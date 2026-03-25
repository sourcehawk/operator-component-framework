package editors

import (
	networkingv1 "k8s.io/api/networking/v1"
)

// IngressSpecEditor provides a typed API for mutating a Kubernetes IngressSpec.
type IngressSpecEditor struct {
	spec *networkingv1.IngressSpec
}

// NewIngressSpecEditor creates a new IngressSpecEditor for the given IngressSpec.
func NewIngressSpecEditor(spec *networkingv1.IngressSpec) *IngressSpecEditor {
	return &IngressSpecEditor{spec: spec}
}

// Raw returns the underlying *networkingv1.IngressSpec.
//
// This is an escape hatch for cases where the typed API is insufficient.
func (e *IngressSpecEditor) Raw() *networkingv1.IngressSpec {
	return e.spec
}

// SetIngressClassName sets the IngressClassName field of the IngressSpec.
func (e *IngressSpecEditor) SetIngressClassName(name string) {
	e.spec.IngressClassName = &name
}

// SetDefaultBackend sets the default backend for the Ingress.
func (e *IngressSpecEditor) SetDefaultBackend(backend *networkingv1.IngressBackend) {
	e.spec.DefaultBackend = backend
}

// EnsureRule upserts a rule by Host. If a rule with the same Host already exists,
// it is replaced. Otherwise the rule is appended.
func (e *IngressSpecEditor) EnsureRule(rule networkingv1.IngressRule) {
	for i, existing := range e.spec.Rules {
		if existing.Host == rule.Host {
			e.spec.Rules[i] = rule
			return
		}
	}
	e.spec.Rules = append(e.spec.Rules, rule)
}

// RemoveRule removes the rule with the given host. It is a no-op if no rule
// with that host exists.
func (e *IngressSpecEditor) RemoveRule(host string) {
	for i, existing := range e.spec.Rules {
		if existing.Host == host {
			e.spec.Rules = append(e.spec.Rules[:i], e.spec.Rules[i+1:]...)
			return
		}
	}
}

// EnsureTLS upserts a TLS entry by the first host in the Hosts slice. If a TLS
// entry with the same first host already exists, it is replaced. Otherwise the
// entry is appended.
func (e *IngressSpecEditor) EnsureTLS(tls networkingv1.IngressTLS) {
	if len(tls.Hosts) == 0 {
		return
	}
	key := tls.Hosts[0]
	for i, existing := range e.spec.TLS {
		if len(existing.Hosts) > 0 && existing.Hosts[0] == key {
			e.spec.TLS[i] = tls
			return
		}
	}
	e.spec.TLS = append(e.spec.TLS, tls)
}

// RemoveTLS removes TLS entries whose first host matches any of the provided hosts.
// It is a no-op for hosts that do not match any existing TLS entry.
func (e *IngressSpecEditor) RemoveTLS(hosts ...string) {
	hostSet := make(map[string]struct{}, len(hosts))
	for _, h := range hosts {
		hostSet[h] = struct{}{}
	}
	filtered := e.spec.TLS[:0]
	for _, tls := range e.spec.TLS {
		if len(tls.Hosts) > 0 {
			if _, match := hostSet[tls.Hosts[0]]; match {
				continue
			}
		}
		filtered = append(filtered, tls)
	}
	e.spec.TLS = filtered
}
