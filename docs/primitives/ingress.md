# Ingress Primitive

The `ingress` primitive is the framework's built-in integration abstraction for managing Kubernetes `Ingress` resources. It integrates with the component lifecycle and provides a structured mutation API for managing rules, TLS configuration, and metadata. For an overview of all built-in primitives, see [Primitives](../primitives.md).

## Capabilities

| Capability              | Detail                                                                                         |
|-------------------------|------------------------------------------------------------------------------------------------|
| **Operational status**  | Reports `OperationPending` until the ingress controller assigns an address, then `Operational` |
| **Suspension**          | No-op by default — Ingress is left in place; backend returns 502/503                           |
| **Mutation pipeline**   | Typed editors for metadata and ingress spec (rules, TLS, class name, default backend)          |
| **Flavors**             | Preserves externally-managed fields (labels, annotations)                                      |

## Building an Ingress Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/ingress"

base := &networkingv1.Ingress{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "web-ingress",
        Namespace: owner.Namespace,
    },
    Spec: networkingv1.IngressSpec{
        IngressClassName: ptr.To("nginx"),
        Rules: []networkingv1.IngressRule{
            {
                Host: "example.com",
                IngressRuleValue: networkingv1.IngressRuleValue{
                    HTTP: &networkingv1.HTTPIngressRuleValue{
                        Paths: []networkingv1.HTTPIngressPath{
                            {
                                Path:     "/",
                                PathType: ptr.To(networkingv1.PathTypePrefix),
                                Backend: networkingv1.IngressBackend{
                                    Service: &networkingv1.IngressServiceBackend{
                                        Name: "web-svc",
                                        Port: networkingv1.ServiceBackendPort{Number: 80},
                                    },
                                },
                            },
                        },
                    },
                },
            },
        },
    },
}

resource, err := ingress.NewBuilder(base).
    WithFieldApplicationFlavor(ingress.PreserveCurrentAnnotations).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying an `Ingress` beyond its baseline. Each mutation is a named function that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) ingress.Mutation {
    return ingress.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *ingress.Mutator) error {
            // record edits here
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by another, register the dependency first.

### Boolean-gated mutations

Use `When(bool)` to gate a mutation on a runtime condition:

```go
func TLSMutation(version string, enabled bool) ingress.Mutation {
    return ingress.Mutation{
        Name:    "tls",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *ingress.Mutator) error {
            m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
                e.EnsureTLS(networkingv1.IngressTLS{
                    Hosts:      []string{"example.com"},
                    SecretName: "tls-cert",
                })
                return nil
            })
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are recorded:

| Step | Category           | What it affects                                         |
|------|--------------------|---------------------------------------------------------|
| 1    | Metadata edits     | Labels and annotations on the `Ingress` object          |
| 2    | Ingress spec edits | Ingress class, default backend, rules, TLS via editor   |

Within each category, edits are applied in their registration order. Later features observe the Ingress as modified by all previous features.

## Editors

### IngressSpecEditor

The primary API for modifying the Ingress spec. Use `m.EditIngressSpec` for full control:

```go
m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
    e.SetIngressClassName("nginx")
    e.EnsureRule(networkingv1.IngressRule{Host: "example.com"})
    e.EnsureTLS(networkingv1.IngressTLS{
        Hosts:      []string{"example.com"},
        SecretName: "tls-cert",
    })
    return nil
})
```

#### SetIngressClassName

Sets the `spec.ingressClassName` field.

#### SetDefaultBackend

Sets the default backend for traffic that does not match any rule.

#### EnsureRule and RemoveRule

`EnsureRule` upserts a rule by `Host` — if a rule with the same host already exists, it is replaced. `RemoveRule` deletes the rule with the given host; it is a no-op if no matching rule exists.

```go
m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
    e.EnsureRule(networkingv1.IngressRule{
        Host: "api.example.com",
        IngressRuleValue: networkingv1.IngressRuleValue{
            HTTP: &networkingv1.HTTPIngressRuleValue{
                Paths: []networkingv1.HTTPIngressPath{
                    {
                        Path:     "/v1",
                        PathType: ptr.To(networkingv1.PathTypePrefix),
                        Backend: networkingv1.IngressBackend{
                            Service: &networkingv1.IngressServiceBackend{
                                Name: "api-svc",
                                Port: networkingv1.ServiceBackendPort{Number: 8080},
                            },
                        },
                    },
                },
            },
        },
    })
    e.RemoveRule("deprecated.example.com")
    return nil
})
```

#### EnsureTLS and RemoveTLS

`EnsureTLS` upserts a TLS entry by the first host in the `Hosts` slice. `RemoveTLS` removes TLS entries whose first host matches any of the provided hosts.

```go
m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
    e.EnsureTLS(networkingv1.IngressTLS{
        Hosts:      []string{"example.com", "www.example.com"},
        SecretName: "wildcard-tls",
    })
    e.RemoveTLS("old.example.com")
    return nil
})
```

#### Raw Escape Hatch

`Raw()` returns the underlying `*networkingv1.IngressSpec` for direct access when the typed API is insufficient:

```go
m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
    spec := e.Raw()
    // direct manipulation
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureAnnotation("nginx.ingress.kubernetes.io/rewrite-target", "/")
    return nil
})
```

## Operational Status

The Ingress primitive uses the **Integration** lifecycle, which implements `concepts.Operational` instead of `concepts.Alive`.

### DefaultOperationalStatusHandler

| Condition                                  | Status        | Reason                                    |
|--------------------------------------------|---------------|-------------------------------------------|
| Entry with `IP != ""` or `Hostname != ""` | `Operational` | Ingress has been assigned an address      |
| Otherwise                                  | `OperationPending` | Awaiting load balancer address assignment |

The handler iterates over `Status.LoadBalancer.Ingress` entries and requires at least one with a non-empty `IP` or `Hostname` to report operational.

Override with `WithCustomOperationalStatus` for more complex health checks (e.g. verifying specific annotations set by cloud providers).

## Suspension

### Default Behaviour

The default suspension strategy is a **no-op**:

- `DefaultDeleteOnSuspendHandler` returns `false` — the Ingress is not deleted.
- `DefaultSuspendMutationHandler` does nothing — the Ingress spec is not modified.
- `DefaultSuspensionStatusHandler` immediately reports `Suspended` with reason `"Ingress suspended (backend unavailable)"`.

**Rationale**: deleting an Ingress causes the ingress controller (e.g. nginx) to reload its configuration, which affects the entire cluster's routing — not just the suspended service. When the backend service is suspended, the Ingress returning 502/503 is the correct observable behaviour.

### Custom Suspension

Override any of the suspension handlers via the builder:

```go
resource, err := ingress.NewBuilder(base).
    WithCustomSuspendDeletionDecision(func(_ *networkingv1.Ingress) bool {
        return true // delete on suspend
    }).
    Build()
```

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

```go
resource, err := ingress.NewBuilder(base).
    WithFieldApplicationFlavor(ingress.PreserveCurrentLabels).
    Build()
```

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on overlap.

This is particularly useful for Ingress resources, where ingress controllers and cert-manager often manage annotations:

```go
resource, err := ingress.NewBuilder(base).
    WithFieldApplicationFlavor(ingress.PreserveCurrentAnnotations).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use `feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for boolean conditions.

**Use `PreserveCurrentAnnotations` when sharing an Ingress.** Ingress controllers, cert-manager, and external-dns frequently manage annotations. This flavor prevents your operator from silently deleting those annotations each reconcile cycle.

**Register mutations in dependency order.** If mutation B relies on a rule added by mutation A, register A first.

**Prefer no-op suspension.** The default no-op suspension is almost always correct for Ingress resources. Only override to delete-on-suspend if your use case specifically requires removing the Ingress from the cluster during suspension.
