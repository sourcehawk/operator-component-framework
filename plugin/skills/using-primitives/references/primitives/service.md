# Service Primitive

The `service` primitive wraps a Kubernetes `Service` and integrates with the component lifecycle as an Integration,
Graceful, and Suspendable resource.

## Capabilities

| Capability            | Detail                                                                                             |
| --------------------- | -------------------------------------------------------------------------------------------------- |
| **Operational**       | Monitors LoadBalancer ingress assignment; reports `Operational` or `OperationPending`              |
| **Graceful**          | LoadBalancer with no ingress reports `Degraded`; non-LoadBalancer or assigned ingress is `Healthy` |
| **Suspendable**       | No-op by default; Service is left in place. Customizable via handlers                              |
| **DataExtractable**   | Reads assigned ClusterIP or LoadBalancer ingress after each sync cycle                             |
| **Mutation pipeline** | Typed editors for metadata and Service spec, with a `Raw()` escape hatch for free-form access      |

See [Lifecycle Interfaces](../primitives.md#lifecycle-interfaces) for the full set of status values each interface
reports.

## Building a Service Primitive

```go
import "github.com/sourcehawk/operator-component-framework/pkg/primitives/service"

base := &corev1.Service{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "app-svc",
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
    WithMutation(BaseServiceMutation(owner.Spec.Version)).
    Build()
```

## Mutations

Register mutations with `WithMutation`. The mutation system, boolean-gated mutations, and version-gated mutations are
explained in [The Mutation System](../primitives.md#the-mutation-system),
[Boolean-Gated Mutations](../primitives.md#boolean-gated-mutations), and
[Version-Gated Mutations](../primitives.md#version-gated-mutations).

A kind-specific example, gating a NodePort mutation on a boolean condition:

```go
func NodePortMutation(version string, enabled bool) service.Mutation {
    return service.Mutation{
        Name:    "nodeport",
        Feature: feature.NewVersionGate(version, nil).When(enabled),
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

## Internal Mutation Ordering

Within a single mutation, edits are applied in a fixed category order regardless of recording order:

| Step | Category       | What it affects                          |
| ---- | -------------- | ---------------------------------------- |
| 1    | Metadata edits | Labels and annotations on the `Service`  |
| 2    | ServiceSpec    | Ports, selectors, type, traffic policies |

Within each category, edits run in registration order. Later features observe the Service as modified by all earlier
ones.

## Relevant Editors

See [Mutation Editors](../primitives.md#mutation-editors) for the general editor model.

### ServiceSpecEditor

Controls Service-level settings via `m.EditServiceSpec`.

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

`EnsurePort` upserts a port: if a port with the same `Name` exists it is replaced; when `Name` is empty the match uses
the combination of `Port` and the effective `Protocol` (treating an empty protocol as TCP). TCP and UDP ports with the
same port number are distinct unless protocols match explicitly. If no existing port matches, the new port is appended.
`RemovePort` removes a port by name.

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
    e.EnsureSelector("app", "web")
    e.EnsureSelector("tier", "frontend")
    return nil
})
```

Use `Raw()` for fields not covered by the typed API:

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

## Data Extraction

`service.ExtractInto` declares that this Service produces the value of a data cell, such as the assigned ClusterIP or
LoadBalancer ingress. The function receives a value copy of the reconciled Service after each sync cycle:

```go
clusterIP := concepts.NewData[string]("backend-cluster-ip")

builder := service.NewBuilder(base)
service.ExtractInto(builder, clusterIP, func(svc corev1.Service) (string, error) {
    return svc.Spec.ClusterIP, nil
})

resource, err := builder.Build()
```

Resources registered later in the same component block on the cell with `WithDataGuard(clusterIP)` or read it
opportunistically with `WithOptionalData(clusterIP)`. See [Declared Data](../component.md#declared-data).

## Operational Status

The Service primitive implements `concepts.Operational`. The default handler reports:

| Service Type   | Condition                                                                   | Status             |
| -------------- | --------------------------------------------------------------------------- | ------------------ |
| `LoadBalancer` | `Status.LoadBalancer.Ingress` has no entry with an IP or hostname           | `OperationPending` |
| `LoadBalancer` | `Status.LoadBalancer.Ingress` has at least one entry with an IP or hostname | `Operational`      |
| `ClusterIP`    | Always                                                                      | `Operational`      |
| `NodePort`     | Always                                                                      | `Operational`      |
| `ExternalName` | Always                                                                      | `Operational`      |
| Headless       | Always                                                                      | `Operational`      |

Override with `WithCustomOperationalStatus`:

```go
resource, err := service.NewBuilder(base).
    WithCustomOperationalStatus(func(op concepts.ConvergingOperation, svc *corev1.Service) (concepts.OperationalStatusWithReason, error) {
        return service.DefaultOperationalStatusHandler(op, svc)
    }).
    Build()
```

## Grace Status

The default grace status handler assesses health after the grace period expires:

| Service Type   | Condition                                 | Status     |
| -------------- | ----------------------------------------- | ---------- |
| `LoadBalancer` | `Status.LoadBalancer.Ingress` has entries | `Healthy`  |
| `LoadBalancer` | `Status.LoadBalancer.Ingress` is empty    | `Degraded` |
| `ClusterIP`    | Always                                    | `Healthy`  |
| `NodePort`     | Always                                    | `Healthy`  |
| `ExternalName` | Always                                    | `Healthy`  |
| Headless       | Always                                    | `Healthy`  |

Override with `WithCustomGraceStatus`:

```go
service.NewBuilder(base).
    WithCustomGraceStatus(func(svc *corev1.Service) (concepts.GraceStatusWithReason, error) {
        status, err := service.DefaultGraceStatusHandler(svc)
        if err != nil {
            return status, err
        }
        // Add custom logic
        return status, nil
    })
```

## Suspension

By default, Services are unaffected by suspension. They remain in the cluster when the parent component is suspended.
`DefaultDeleteOnSuspendHandler` returns `false`, `DefaultSuspendMutationHandler` is a no-op, and
`DefaultSuspensionStatusHandler` reports `Suspended` immediately.

This is appropriate for most use cases because Services are stateless routing objects that are safe to leave in place.

Override with `WithCustomSuspendDeletionDecision` to delete the Service on suspend:

```go
resource, err := service.NewBuilder(base).
    WithCustomSuspendDeletionDecision(func(_ *corev1.Service) bool {
        return true
    }).
    Build()
```

Combine `WithCustomSuspendMutation` and `WithCustomSuspendStatus` for more advanced suspension behavior, such as
modifying the Service before deletion or tracking external readiness before reporting suspended.

## Full Example

```go
func BaseServiceMutation(version string) service.Mutation {
    return service.Mutation{
        Name:    "base-service",
        Feature: feature.NewVersionGate(version, nil),
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
        Feature: feature.NewVersionGate(version, nil).When(enabled),
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

clusterIP := concepts.NewData[string]("backend-cluster-ip")

builder := service.NewBuilder(base).
    WithMutation(BaseServiceMutation(owner.Spec.Version)).
    WithMutation(MetricsPortMutation(owner.Spec.Version, owner.Spec.EnableMetrics))

service.ExtractInto(builder, clusterIP, func(svc corev1.Service) (string, error) {
    return svc.Spec.ClusterIP, nil
})

resource, err := builder.Build()
```

When `EnableMetrics` is true, the Service exposes both the HTTP and metrics ports. When false, only HTTP is configured.

## Guidance

**`Feature: nil` applies unconditionally.** Omit `Feature` for mutations that should always run. Chain `.When(bool)` for
boolean conditions and pass version constraints to `NewVersionGate` for version-gated behavior.

**Register mutations in dependency order.** If mutation B depends on a port added by mutation A, register A first.

**Use `EnsurePort` for idempotent port management.** Ports are tracked by name (or port number when unnamed), so
repeated calls with the same name produce the same result.

**Leave Services in place during suspension.** The no-op default is correct for most Services. Only override
`WithCustomSuspendDeletionDecision` when your use case requires explicitly removing the Service during suspension.

**Use `ExtractInto` for assigned addresses.** ClusterIP and LoadBalancer ingress are server-assigned. Declare an
extraction into a data cell rather than caching them in mutation logic.
