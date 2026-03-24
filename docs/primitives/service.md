# Service Primitive

The `service` primitive is the framework's built-in integration abstraction for managing Kubernetes `Service` resources.
It integrates with the component lifecycle and provides a structured mutation API for managing ports, selectors, and
service configuration.

## Capabilities

| Capability               | Detail                                                                                        |
| ------------------------ | --------------------------------------------------------------------------------------------- |
| **Operational tracking** | Monitors LoadBalancer ingress assignment; reports `Operational` or `Pending`                  |
| **Suspension**           | Unaffected by suspension by default; customizable via handlers to delete or mutate on suspend |
| **Mutation pipeline**    | Typed editors for metadata and service spec, with a raw escape hatch for free-form access     |
| **Flavors**              | Preserves externally-managed fields (labels, annotations)                                     |
| **Data extraction**      | Reads generated or updated values (ClusterIP, LoadBalancer ingress) after each sync cycle     |

## Building a Service Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/service"

base := &corev1.Service{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "app-service",
        Namespace: owner.Namespace,
    },
    Spec: corev1.ServiceSpec{
        Selector: map[string]string{"app": owner.Name},
        Ports: []corev1.ServicePort{
            {Name: "http", Port: 80, TargetPort: intstr.FromInt32(8080)},
        },
    },
}

resource, err := service.NewBuilder(base).
    WithFieldApplicationFlavor(service.PreserveCurrentAnnotations).
    WithMutation(MyFeatureMutation(owner.Spec.Version)).
    Build()
```

## Default Field Application

`DefaultFieldApplicator` replaces the current Service with a deep copy of the desired object while preserving
server-managed metadata, the Status subresource (including `LoadBalancer.Ingress`), the immutable `spec.clusterIP` and
`spec.clusterIPs` fields that Kubernetes assigns after creation, server-defaulted IP family configuration
(`spec.ipFamilies` and `spec.ipFamilyPolicy`), the server-allocated `spec.healthCheckNodePort`, and auto-allocated
NodePort values for `NodePort` and `LoadBalancer` Services.

This prevents the operator from clearing the cluster-assigned IP, load balancer ingress assignments, IP family defaults,
health check node ports, or automatically-assigned NodePort numbers on every reconciliation cycle while ensuring all
other fields converge to the desired state. Explicitly specified values in the desired object always take precedence, so
these fields can still be managed by the operator when needed.

Use `WithCustomFieldApplicator` when other controllers manage fields that should not be overwritten:

```go
resource, err := service.NewBuilder(base).
    WithCustomFieldApplicator(func(current, desired *corev1.Service) error {
        // Only synchronise owned fields; leave other fields untouched.
        current.Spec.Ports = desired.DeepCopy().Spec.Ports
        return nil
    }).
    Build()
```

## Mutations

Mutations are the primary mechanism for modifying a `Service` beyond its baseline. Each mutation is a named function
that receives a `*Mutator` and records edit intent through typed editors.

The `Feature` field controls when a mutation applies. Leaving it nil applies the mutation unconditionally. A feature
with no version constraints and no `When()` conditions is also always enabled:

```go
func MyFeatureMutation(version string) service.Mutation {
    return service.Mutation{
        Name:    "my-feature",
        Feature: feature.NewResourceFeature(version, nil), // always enabled
        Mutate: func(m *service.Mutator) error {
            // record edits here
            return nil
        },
    }
}
```

Mutations are applied in the order they are registered with the builder. If one mutation depends on a change made by
another, register the dependency first.

### Boolean-gated mutations

Use `When(bool)` to gate a mutation on a runtime condition:

```go
func NodePortMutation(version string, enabled bool) service.Mutation {
    return service.Mutation{
        Name:    "nodeport",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *service.Mutator) error {
            m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
                e.SetType(corev1.ServiceTypeNodePort)
                return nil
            })
            return nil
        },
    }
}
```

### Version-gated mutations

Pass a `[]feature.VersionConstraint` to gate on a semver range:

```go
var legacyConstraint = mustSemverConstraint("< 2.0.0")

func LegacyPortMutation(version string) service.Mutation {
    return service.Mutation{
        Name: "legacy-port",
        Feature: feature.NewResourceFeature(
            version,
            []feature.VersionConstraint{legacyConstraint},
        ),
        Mutate: func(m *service.Mutator) error {
            m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
                e.EnsurePort(corev1.ServicePort{Name: "legacy", Port: 9090})
                return nil
            })
            return nil
        },
    }
}
```

All version constraints and `When()` conditions must be satisfied for a mutation to apply.

## Internal Mutation Ordering

Within a single mutation, edit operations are grouped into categories and applied in a fixed sequence regardless of the
order they are recorded:

| Step | Category          | What it affects                          |
| ---- | ----------------- | ---------------------------------------- |
| 1    | Metadata edits    | Labels and annotations on the `Service`  |
| 2    | ServiceSpec edits | Ports, selectors, type, traffic policies |

Within each category, edits are applied in their registration order. Later features observe the Service as modified by
all previous features.

## Editors

### ServiceSpecEditor

Controls service-level settings via `m.EditServiceSpec`.

Available methods: `SetType`, `EnsurePort`, `RemovePort`, `SetSelector`, `EnsureSelector`, `RemoveSelector`,
`SetSessionAffinity`, `SetSessionAffinityConfig`, `SetPublishNotReadyAddresses`, `SetExternalTrafficPolicy`,
`SetInternalTrafficPolicy`, `SetLoadBalancerSourceRanges`, `SetExternalName`, `Raw`.

```go
m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
    e.SetType(corev1.ServiceTypeLoadBalancer)
    e.EnsurePort(corev1.ServicePort{
        Name:       "https",
        Port:       443,
        TargetPort: intstr.FromInt32(8443),
    })
    e.SetExternalTrafficPolicy(corev1.ServiceExternalTrafficPolicyLocal)
    return nil
})
```

#### Port Management

`EnsurePort` upserts a port: if a port with the same `Name` exists, it is replaced; otherwise, when `Name` is empty, the
match is performed on the combination of `Port` and the effective `Protocol` (treating an empty protocol value as TCP).
This means TCP and UDP ports with the same port number are considered distinct unless you explicitly set matching
protocols. If no existing port matches, the new port is appended. `RemovePort` removes a port by name.

```go
m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
    e.EnsurePort(corev1.ServicePort{Name: "http", Port: 80})
    e.RemovePort("legacy")
    return nil
})
```

#### Selector Management

`SetSelector` replaces the entire selector map. `EnsureSelector` adds or updates a single key-value pair.
`RemoveSelector` removes a single key.

```go
m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
    e.EnsureSelector("app", "myapp")
    e.EnsureSelector("env", "production")
    return nil
})
```

For fields not covered by the typed API, use `Raw()`:

```go
m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
    e.Raw().HealthCheckNodePort = 30000
    return nil
})
```

### ObjectMetaEditor

Modifies labels and annotations via `m.EditObjectMetadata`.

Available methods: `EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`.

```go
m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
    e.EnsureLabel("app.kubernetes.io/version", version)
    e.EnsureAnnotation("service.beta.kubernetes.io/aws-load-balancer-type", "nlb")
    return nil
})
```

## Flavors

Flavors run after the baseline applicator and before mutations. They are used to preserve fields managed by external
controllers or other tools.

### PreserveCurrentLabels

Preserves labels present on the live object but absent from the applied desired state. Applied labels win on overlap.

```go
resource, err := service.NewBuilder(base).
    WithFieldApplicationFlavor(service.PreserveCurrentLabels).
    Build()
```

### PreserveCurrentAnnotations

Preserves annotations present on the live object but absent from the applied desired state. Applied annotations win on
overlap.

```go
resource, err := service.NewBuilder(base).
    WithFieldApplicationFlavor(service.PreserveCurrentAnnotations).
    Build()
```

Multiple flavors can be registered and run in registration order.

## Operational Status

The Service primitive implements the `Operational` concept to track whether the Service is ready to accept traffic.

### DefaultOperationalStatusHandler

| Service Type   | Behaviour                                                                                                    |
| -------------- | ------------------------------------------------------------------------------------------------------------ |
| `LoadBalancer` | Reports `Pending` until `Status.LoadBalancer.Ingress` has entries with an IP or hostname; then `Operational` |
| `ClusterIP`    | Immediately `Operational`                                                                                    |
| `NodePort`     | Immediately `Operational`                                                                                    |
| `ExternalName` | Immediately `Operational`                                                                                    |
| Headless       | Immediately `Operational`                                                                                    |

Override with `WithCustomOperationalStatus` to add custom checks:

```go
resource, err := service.NewBuilder(base).
    WithCustomOperationalStatus(func(op concepts.ConvergingOperation, svc *corev1.Service) (concepts.OperationalStatusWithReason, error) {
        // Custom logic, e.g. check for specific annotations
        return service.DefaultOperationalStatusHandler(op, svc)
    }).
    Build()
```

## Suspension

By default, Services are **unaffected** by suspension — they remain in the cluster when the parent component is
suspended. The default suspend mutation handler is a no-op, `DefaultDeleteOnSuspendHandler` returns `false`, and the
default suspension status handler reports `Suspended` immediately (no work required).

This is appropriate for most use cases because Services are stateless routing objects that are safe to leave in place.

Override with `WithCustomSuspendDeletionDecision` if you want to delete the Service on suspend:

```go
resource, err := service.NewBuilder(base).
    WithCustomSuspendDeletionDecision(func(_ *corev1.Service) bool {
        return true // delete the Service during suspension
    }).
    Build()
```

You can also combine `WithCustomSuspendMutation` and `WithCustomSuspendStatus` for more advanced suspension behaviour,
such as modifying the Service before it is deleted or tracking external readiness before reporting suspended.

## Data Extraction

Use `WithDataExtractor` to read values from the reconciled Service, such as the assigned ClusterIP or LoadBalancer
ingress:

```go
var assignedIP string

resource, err := service.NewBuilder(base).
    WithDataExtractor(func(svc corev1.Service) error {
        assignedIP = svc.Spec.ClusterIP
        return nil
    }).
    Build()
```

## Full Example: Feature-Composed Service

```go
func BaseServiceMutation(version string) service.Mutation {
    return service.Mutation{
        Name:    "base-service",
        Feature: feature.NewResourceFeature(version, nil),
        Mutate: func(m *service.Mutator) error {
            m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
                e.EnsurePort(corev1.ServicePort{
                    Name:       "http",
                    Port:       80,
                    TargetPort: intstr.FromInt32(8080),
                })
                return nil
            })
            return nil
        },
    }
}

func MetricsPortMutation(version string, enabled bool) service.Mutation {
    return service.Mutation{
        Name:    "metrics-port",
        Feature: feature.NewResourceFeature(version, nil).When(enabled),
        Mutate: func(m *service.Mutator) error {
            m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
                e.EnsurePort(corev1.ServicePort{
                    Name:       "metrics",
                    Port:       9090,
                    TargetPort: intstr.FromInt32(9090),
                })
                return nil
            })
            return nil
        },
    }
}

resource, err := service.NewBuilder(base).
    WithFieldApplicationFlavor(service.PreserveCurrentAnnotations).
    WithMutation(BaseServiceMutation(owner.Spec.Version)).
    WithMutation(MetricsPortMutation(owner.Spec.Version, owner.Spec.EnableMetrics)).
    Build()
```

When `EnableMetrics` is true, the Service will expose both the HTTP port and the metrics port. When false, only the
HTTP port is configured. Neither mutation needs to know about the other.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` (leave it nil) for mutations that should always run. Use
`feature.NewResourceFeature(version, constraints)` when version-based gating is needed, and chain `.When(bool)` for
boolean conditions.

**Register mutations in dependency order.** If mutation B relies on a port added by mutation A, register A first.

**Use `EnsurePort` for idempotent port management.** The mutator tracks ports by name (or port number when unnamed), so
repeated calls with the same name produce the same result.

**Use `PreserveCurrentAnnotations` when cloud providers annotate Services.** Cloud load balancer controllers often add
annotations to Services. This flavor prevents your operator from deleting those annotations each reconcile cycle.
