# Ingress Primitive

The `ingress` primitive wraps a Kubernetes `Ingress` and integrates with the component lifecycle as an Integration,
Graceful, and Suspendable resource, providing a structured mutation API for managing rules, TLS configuration, and
metadata.

## Capabilities

| Capability            | Detail                                                                                               |
| --------------------- | ---------------------------------------------------------------------------------------------------- |
| **Operational**       | Reports `OperationPending` until the ingress controller assigns an address, then `Operational`       |
| **Graceful**          | Reports `Degraded` until a load balancer IP or hostname is assigned, then `Healthy`                  |
| **Suspendable**       | No-op by default. Ingress is left in place; backend returns 502/503 when the backing service is down |
| **DataExtractable**   | Reads assigned load balancer addresses after each sync cycle via `ExtractInto`                       |
| **Mutation pipeline** | Typed editors for metadata and Ingress spec (rules, TLS, class name, default backend)                |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full set of status values each interface
reports.

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
                Host: "app.example.com",
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
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Register mutations with `WithMutation`. The mutation system, boolean-gated mutations, and version-gated mutations are
explained in [The Mutation System](../primitives.md#the-mutation-system),
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations), and
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

A kind-specific example gating a TLS mutation on a boolean condition:

```go
func TLSMutation(version string, enabled bool) ingress.Mutation {
    return ingress.Mutation{
        Name:    "tls",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
        Mutate: func(m *ingress.Mutator) error {
            m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
                e.EnsureTLS(networkingv1.IngressTLS{
                    Hosts:      []string{"app.example.com"},
                    SecretName: "tls-cert",
                })
                return nil
            })
            return nil
        },
    }
}
```

## Internal Mutation Ordering

Within a single mutation, edits are applied in a fixed category order regardless of recording order:

| Step | Category           | What it affects                                       |
| ---- | ------------------ | ----------------------------------------------------- |
| 1    | Metadata edits     | Labels and annotations on the `Ingress` object        |
| 2    | Ingress spec edits | Ingress class, default backend, rules, TLS via editor |

Within each category, edits run in registration order. Later features observe the Ingress as modified by all earlier
ones.

## Relevant Editors

See [Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

### IngressSpecEditor

The primary API for modifying the Ingress spec. Use `m.EditIngressSpec` for full control:

```go
m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
    e.SetIngressClassName("nginx")
    e.EnsureRule(networkingv1.IngressRule{Host: "app.example.com"})
    e.EnsureTLS(networkingv1.IngressTLS{
        Hosts:      []string{"app.example.com"},
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

`EnsureRule` upserts a rule by `Host`. If a rule with the same host already exists, it is replaced. `RemoveRule` deletes
the rule with the given host; it is a no-op if no matching rule exists.

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

`EnsureTLS` upserts a TLS entry by the first host in the `Hosts` slice. `RemoveTLS` removes TLS entries whose first host
matches any of the provided hosts.

```go
m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
    e.EnsureTLS(networkingv1.IngressTLS{
        Hosts:      []string{"app.example.com", "www.example.com"},
        SecretName: "wildcard-tls",
    })
    e.RemoveTLS("old.example.com")
    return nil
})
```

#### Raw Escape Hatch

`Raw()` returns the underlying `*networkingv1.IngressSpec` for direct access:

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

The Ingress primitive implements `concepts.Operational`. The default handler iterates over `Status.LoadBalancer.Ingress`
entries and requires at least one with a non-empty `IP` or `Hostname`:

| Condition                                 | Status             |
| ----------------------------------------- | ------------------ |
| Entry with `IP != ""` or `Hostname != ""` | `Operational`      |
| Otherwise                                 | `OperationPending` |

Override with `WithCustomOperationalStatus` for more complex health checks, such as verifying specific annotations set
by cloud providers.

## Grace Status

The default grace status handler inspects `Status.LoadBalancer.Ingress` after the grace period expires:

| Condition                                                | Status     |
| -------------------------------------------------------- | ---------- |
| At least one entry with a non-empty `IP` or `Hostname`   | `Healthy`  |
| No entries, or all entries lack both `IP` and `Hostname` | `Degraded` |

Override with `WithCustomGraceStatus`:

```go
ingress.NewBuilder(base).
    WithCustomGraceStatus(func(ing *networkingv1.Ingress) (concepts.GraceStatusWithReason, error) {
        status, err := ingress.DefaultGraceStatusHandler(ing)
        if err != nil {
            return status, err
        }
        // Add custom logic
        return status, nil
    })
```

## Suspension

### Default Behavior

The default suspension strategy is a no-op:

- `DefaultDeleteOnSuspendHandler` returns `false`. The Ingress is not deleted.
- `DefaultSuspendMutationHandler` does nothing. The Ingress spec is not modified.
- `DefaultSuspensionStatusHandler` immediately reports `Suspended` with reason
  `"Ingress suspended (backend unavailable)"`.

**Rationale**: deleting an Ingress causes the ingress controller to reload its configuration, which affects the entire
cluster's routing, not just the suspended service. When the backend service is suspended, the Ingress returning 502/503
is the correct observable behavior.

### Custom Suspension

Override any of the suspension handlers via the builder:

```go
resource, err := ingress.NewBuilder(base).
    WithCustomSuspendDeletionDecision(func(_ *networkingv1.Ingress) bool {
        return true // delete on suspend
    }).
    Build()
```

## Full Example

```go
func BaseIngressMutation(version string) ingress.Mutation {
    return ingress.Mutation{
        Name:    "base-ingress",
        Feature: feature.NewVersionGate(version, nil),
        Mutate: func(m *ingress.Mutator) error {
            m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
                e.SetIngressClassName("nginx")
                e.EnsureRule(networkingv1.IngressRule{
                    Host: "app.example.com",
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
                })
                return nil
            })
            return nil
        },
    }
}

func TLSMutation(version string, enabled bool) ingress.Mutation {
    return ingress.Mutation{
        Name:    "tls",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
        Mutate: func(m *ingress.Mutator) error {
            m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
                e.EnsureTLS(networkingv1.IngressTLS{
                    Hosts:      []string{"app.example.com"},
                    SecretName: "tls-cert",
                })
                return nil
            })
            return nil
        },
    }
}

resource, err := ingress.NewBuilder(base).
    WithMutation(BaseIngressMutation(owner.Spec.Version)).
    WithMutation(TLSMutation(owner.Spec.Version, owner.Spec.TLSEnabled)).
    Build()
```

When `TLSEnabled` is true, the Ingress includes a TLS block for the host. When false, only the rule is present.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Use
`feature.NewVersionGate(version, constraints)` for version-based gating and chain `.When(bool)` for boolean conditions.

**Register mutations in dependency order.** If mutation B relies on a rule added by mutation A, register A first.

**Prefer no-op suspension.** The default no-op suspension is almost always correct for Ingress resources. Only override
to delete-on-suspend if your use case specifically requires removing the Ingress from the cluster during suspension.

**Use `EnsureRule` for idempotent rule management.** Rules are matched by `Host`; repeated calls with the same host
replace the existing rule rather than duplicating it.
