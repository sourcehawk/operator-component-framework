# NetworkPolicy Primitive

The `networkpolicy` primitive is the framework's built-in static abstraction for managing Kubernetes `NetworkPolicy`
resources. It integrates with the component lifecycle and provides a structured mutation API for managing pod selectors,
ingress rules, egress rules, and policy types.

## Capabilities

| Capability            | Detail                                                                                                     |
| --------------------- | ---------------------------------------------------------------------------------------------------------- |
| **Static lifecycle**  | No health tracking, grace periods, or suspension. The resource is reconciled to desired state              |
| **Mutation pipeline** | Typed editors for NetworkPolicy spec and object metadata, with a raw escape hatch for free-form access     |
| **Append semantics**  | Ingress and egress rules have no unique key. `AppendIngressRule`/`AppendEgressRule` append unconditionally |
| **Data extraction**   | Reads generated or updated values back from the reconciled NetworkPolicy after each sync cycle             |

## Building a NetworkPolicy Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/networkpolicy"

base := &networkingv1.NetworkPolicy{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "app-netpol",
        Namespace: owner.Namespace,
    },
    Spec: networkingv1.NetworkPolicySpec{
        PodSelector: metav1.LabelSelector{
            MatchLabels: map[string]string{"app": owner.Name},
        },
        PolicyTypes: []networkingv1.PolicyType{
            networkingv1.PolicyTypeIngress,
            networkingv1.PolicyTypeEgress,
        },
    },
}

resource, err := networkpolicy.NewBuilder(base).
    WithMutation(HTTPIngressMutation()).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `NetworkPolicy` beyond its baseline. Each mutation is a named
function that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. Prefer this
for mutations that should always run and do not need feature-gate evaluation:

```go
func HTTPIngressMutation() networkpolicy.Mutation {
    return networkpolicy.Mutation{
        Name: "http-ingress",
        // Feature is nil: mutation is applied unconditionally.
        Mutate: func(m *networkpolicy.Mutator) error {
            m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
                port := intstr.FromInt32(8080)
                tcp := corev1.ProtocolTCP
                e.AppendIngressRule(networkingv1.NetworkPolicyIngressRule{
                    Ports: []networkingv1.NetworkPolicyPort{
                        {Protocol: &tcp, Port: &port},
                    },
                })
                return nil
            })
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by
another, register the dependency first.

### Boolean-gated mutations

```go
func MetricsIngressMutation(version string, enableMetrics bool) networkpolicy.Mutation {
    return networkpolicy.Mutation{
        Name:    "metrics-ingress",
        Feature: feature.NewResourceFeature(version, nil).When(enableMetrics),
        Mutate: func(m *networkpolicy.Mutator) error {
            m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
                port := intstr.FromInt32(9090)
                tcp := corev1.ProtocolTCP
                e.AppendIngressRule(networkingv1.NetworkPolicyIngressRule{
                    Ports: []networkingv1.NetworkPolicyPort{
                        {Protocol: &tcp, Port: &port},
                    },
                })
                return nil
            })
            return nil
        },
    }
}
```

### Version-gated mutations

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyNetworkPolicyMutation(version string) networkpolicy.Mutation {
    return networkpolicy.Mutation{
        Name: "legacy-policy",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *networkpolicy.Mutator) error {
            m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
                e.SetPolicyTypes([]networkingv1.PolicyType{
                    networkingv1.PolicyTypeIngress,
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

Within a single mutation, edit operations are applied in a fixed category order regardless of the order they are
recorded:

| Step | Category       | What it affects                                                 |
| ---- | -------------- | --------------------------------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `NetworkPolicy`                   |
| 2    | Spec edits     | Pod selector, ingress rules, egress rules, policy types via Raw |

Within each category, edits are applied in their registration order. Later features observe the NetworkPolicy as
modified by all previous features.

## Relevant Editors

### NetworkPolicySpecEditor

The primary API for modifying the NetworkPolicy spec. Use `m.EditNetworkPolicySpec` for full control:

```go
m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
    e.SetPodSelector(metav1.LabelSelector{
        MatchLabels: map[string]string{"app": "web"},
    })
    port := intstr.FromInt32(80)
    tcp := corev1.ProtocolTCP
    e.AppendIngressRule(networkingv1.NetworkPolicyIngressRule{
        Ports: []networkingv1.NetworkPolicyPort{
            {Protocol: &tcp, Port: &port},
        },
    })
    return nil
})
```

#### SetPodSelector

Sets the pod selector that determines which pods the policy applies to within the namespace. An empty `LabelSelector`
matches all pods.

#### AppendIngressRule and AppendEgressRule

Append a rule unconditionally. Ingress and egress rules have no unique key, so these methods always append. To replace
the full set of rules atomically, call `RemoveIngressRules` or `RemoveEgressRules` first:

```go
m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
    // Replace all ingress rules atomically.
    e.RemoveIngressRules()
    e.AppendIngressRule(newRule1)
    e.AppendIngressRule(newRule2)
    return nil
})
```

#### RemoveIngressRules and RemoveEgressRules

Clear all ingress or egress rules respectively. Use before `AppendIngressRule`/`AppendEgressRule` to replace the full
set atomically.

#### SetPolicyTypes

Sets the policy types. Valid values are `networkingv1.PolicyTypeIngress` and `networkingv1.PolicyTypeEgress`. When
`Egress` is included, egress rules must be set explicitly to permit traffic; an empty list denies all egress.

#### Raw Escape Hatch

`Raw()` returns the underlying `*networkingv1.NetworkPolicySpec` for free-form editing when none of the structured
methods are sufficient:

```go
m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
    raw := e.Raw()
    if raw.PodSelector.MatchLabels == nil {
        raw.PodSelector.MatchLabels = make(map[string]string)
    }
    raw.PodSelector.MatchLabels["role"] = "db"
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    e.EnsureAnnotation("policy/managed-by", "operator")
    return nil
})
```

## Full Example: Feature-Composed Network Policy

```go
func HTTPIngressMutation() networkpolicy.Mutation {
    return networkpolicy.Mutation{
        Name: "http-ingress",
        Mutate: func(m *networkpolicy.Mutator) error {
            m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
                port := intstr.FromInt32(8080)
                tcp := corev1.ProtocolTCP
                e.AppendIngressRule(networkingv1.NetworkPolicyIngressRule{
                    Ports: []networkingv1.NetworkPolicyPort{
                        {Protocol: &tcp, Port: &port},
                    },
                })
                return nil
            })
            return nil
        },
    }
}

func MetricsIngressMutation(version string, enabled bool) networkpolicy.Mutation {
    return networkpolicy.Mutation{
        Name:    "metrics-ingress",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *networkpolicy.Mutator) error {
            m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
                port := intstr.FromInt32(9090)
                tcp := corev1.ProtocolTCP
                e.AppendIngressRule(networkingv1.NetworkPolicyIngressRule{
                    Ports: []networkingv1.NetworkPolicyPort{
                        {Protocol: &tcp, Port: &port},
                    },
                })
                return nil
            })
            return nil
        },
    }
}

resource, err := networkpolicy.NewBuilder(base).
    WithMutation(HTTPIngressMutation()).
    WithMutation(MetricsIngressMutation(owner.Spec.Version, owner.Spec.EnableMetrics)).
    Build()
```

When `EnableMetrics` is true, the final NetworkPolicy will have both HTTP and metrics ingress rules. When false, only
the HTTP rule is present. Neither mutation needs to know about the other.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Use `RemoveIngressRules`/`RemoveEgressRules` for atomic replacement.** Since rules have no unique key, there is no
upsert-by-key operation. To replace the full set of rules, call `Remove*Rules` first and then add the desired rules.
Alternatively, use `Raw()` for fine-grained manipulation.

**Register mutations in dependency order.** If mutation B relies on a rule added by mutation A, register A first. Since
`AppendIngressRule`/`AppendEgressRule` append unconditionally, the order of registration determines the order of rules
in the resulting spec.
