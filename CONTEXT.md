# Primitive Implementation Context

This document provides everything needed to implement a new Kubernetes primitive in this framework from scratch, with no prior context. Read it fully before writing any code.

---

## Module and Import Paths

```
module github.com/sourcehawk/operator-component-framework
go 1.25.6
```

Key packages:
- `github.com/sourcehawk/operator-component-framework/internal/generic` — generic base types (internal; primitives use these directly)
- `github.com/sourcehawk/operator-component-framework/pkg/component/concepts` — lifecycle interface contracts and status types
- `github.com/sourcehawk/operator-component-framework/pkg/feature` — `Mutation[T]`, `ResourceFeature`
- `github.com/sourcehawk/operator-component-framework/pkg/mutation/editors` — typed mutation editors
- `github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors` — container selectors
- `github.com/sourcehawk/operator-component-framework/pkg/flavors` — `FieldApplicationFlavor[T]`, generic `PreserveCurrentLabels`, `PreserveCurrentAnnotations`
- `github.com/sourcehawk/operator-component-framework/pkg/flavors/utils` — `PreserveMap`

---

## Architecture Overview

```
Controller
  └─ Component                       (pkg/component)
      └─ Resource Primitive           (pkg/primitives/<kind>)
           └─ generic.*Resource       (internal/generic)
                └─ Kubernetes Object
```

A **primitive** is a package under `pkg/primitives/<kind>/` that wraps one Kubernetes object kind and exposes it as a `component.Resource`. Primitives also implement optional lifecycle interfaces depending on their category.

---

## Primitive Categories

Each primitive belongs to exactly one category. The category determines which `internal/generic` types it uses and which lifecycle interfaces the `Resource` implements.

| Category    | Kubernetes Kinds                                            | Generic base              | Lifecycle interfaces implemented by Resource           |
|-------------|-------------------------------------------------------------|---------------------------|--------------------------------------------------------|
| **Static**  | ConfigMap, Secret, ServiceAccount, RBAC, PodDisruptionBudget | `StaticResource[T, M]`    | `component.Resource`, optionally `DataExtractable`     |
| **Workload** | Deployment, StatefulSet, DaemonSet                         | `WorkloadResource[T, M]`  | `component.Resource`, `Alive`, `Graceful`, `Suspendable`, optionally `DataExtractable` |
| **Task**    | Job                                                         | `TaskResource[T, M]`      | `component.Resource`, `Completable`, `Suspendable`, optionally `DataExtractable` |
| **Integration** | Service, Ingress, Gateway, CronJob                      | `IntegrationResource[T, M]` | `component.Resource`, `Operational`, optionally `Suspendable`, `DataExtractable` |

If a resource does not implement `Alive`, `Completable`, or `Operational`, the component layer considers it `Ready` as long as it exists in the cluster.

---

## Lifecycle Interface Contracts

### `component.Resource` (always required)

```go
// pkg/component/resource.go
type Resource interface {
    Object() (client.Object, error)
    Mutate(current client.Object) error
    Identity() string
}
```

### `concepts.Alive` (Workload)

```go
type Alive interface {
    ConvergingStatus(op ConvergingOperation) (AliveStatusWithReason, error)
}
// Status values: AliveConvergingStatusHealthy | Creating | Updating | Scaling | Failing
```

### `concepts.Graceful` (Workload)

```go
type Graceful interface {
    GraceStatus() (GraceStatusWithReason, error)
}
// Status values: GraceStatusHealthy | GraceStatusDegraded | GraceStatusDown
```

### `concepts.Suspendable` (Workload, Task, optionally Integration)

```go
type Suspendable interface {
    DeleteOnSuspend() bool
    Suspend() error
    SuspensionStatus() (SuspensionStatusWithReason, error)
}
// Status values: SuspensionStatusPending | SuspensionStatusSuspending | SuspensionStatusSuspended
```

### `concepts.Completable` (Task)

```go
type Completable interface {
    ConvergingStatus(op ConvergingOperation) (CompletionStatusWithReason, error)
}
// Status values: CompletionStatusCompleted | CompletionStatusRunning | CompletionStatusPending | CompletionStatusFailing
```

### `concepts.Operational` (Integration)

```go
type Operational interface {
    ConvergingStatus(op ConvergingOperation) (OperationalStatusWithReason, error)
}
// Status values: OperationalStatusOperational | OperationalStatusPending | OperationalStatusFailing
```

### `concepts.DataExtractable` (all categories, optional)

```go
type DataExtractable interface {
    ExtractData() error
}
```

---

## Generic Builder and Resource Types

All `internal/generic` builders and resources are parameterized on `T client.Object` (the Kubernetes type) and `M MutatorApplier` (the primitive's `Mutator`).

### `MutatorApplier` interface

```go
// internal/generic/resource_workload.go
type MutatorApplier interface {
    Apply() error
}
```

Additionally, any Mutator that participates in feature boundaries must also implement `beginFeature()` (unexported). This is satisfied by embedding it in the concrete Mutator and calling it in `NewMutator`.

### Static

```go
// Builder
generic.NewStaticBuilder[T, M](obj T, identityFunc func(T) string, defaultApplicator generic.FieldApplicator[T], newMutator func(T) M) *StaticBuilder[T, M]

// Build() returns *StaticResource[T, M]
// StaticResource embeds BaseResource[T, M]
```

### Workload

```go
generic.NewWorkloadBuilder[T, M](...) *WorkloadBuilder[T, M]
// Additional methods: WithCustomConvergeStatus, WithCustomGraceStatus, WithCustomSuspendStatus, WithCustomSuspendMutation, WithCustomSuspendDeletionDecision
// Build() returns *WorkloadResource[T, M]
```

### Task

```go
generic.NewTaskBuilder[T, M](...) *TaskBuilder[T, M]
// Additional methods: WithCustomConvergeStatus, WithCustomSuspendStatus, WithCustomSuspendMutation, WithCustomSuspendDeletionDecision
// Build() returns *TaskResource[T, M]
```

### Integration

```go
generic.NewIntegrationBuilder[T, M](...) *IntegrationBuilder[T, M]
// Additional methods: WithCustomOperationalStatus, WithCustomSuspendStatus, WithCustomSuspendMutation, WithCustomSuspendDeletionDecision
// Build() returns *IntegrationResource[T, M]
```

### Common builder methods (all categories)

```go
.WithMutation(m Mutation[M])
.WithCustomFieldApplicator(applicator FieldApplicator[T])
.WithFieldApplicationFlavor(flavor FieldApplicationFlavor[T])
.WithDataExtractor(extractor func(T) error)
```

---

## File Layout: The Canonical Pattern

Every primitive lives in `pkg/primitives/<kind>/` and consists of exactly **5 files**:

```
pkg/primitives/<kind>/
  mutator.go        — package doc, Mutation type alias, featurePlan struct, Mutator struct, NewMutator, beginFeature, edit methods, Apply()
  resource.go       — DefaultFieldApplicator, Resource struct, all interface methods delegating to base
  builder.go        — package doc comment, Builder struct, NewBuilder, all WithX methods, Build()
  flavors.go        — FieldApplicationFlavor type alias, kind-specific flavor functions
  handlers.go       — Default*Handler functions (status, suspend, etc.) — only for Workload, Task, Integration categories
```

**Static primitives do not have a `handlers.go`** because they have no status or suspension logic.

Additionally, each file that is the package entry point (typically `mutator.go`) carries the Go package doc comment.

---

## Canonical Example: `pkg/primitives/configmap` (Static)

This is the cleanest reference for new implementations because it is the simplest category. Use it as the baseline, then add lifecycle complexity for Workload/Task/Integration.

**Why it is the best reference:**
- Smallest surface area — no status handlers, no suspension
- Shows the complete plan-and-apply Mutator pattern
- Shows the `FieldApplicationFlavor` type alias pattern
- Shows how `DataExtractable` is wired (value-copy extractor signature: `func(T) error` wrapping `func(*T) error`)
- Tests are comprehensive and show the full test surface required

---

## Implementation Recipe

### Step 1: `mutator.go`

1. Add `// Package <kind> provides...` doc comment on the package declaration.
2. Define `type Mutation feature.Mutation[*Mutator]` — the public mutation alias.
3. Define a `featurePlan` struct with one field per edit category (e.g. `metadataEdits []func(*editors.ObjectMetaEditor) error`).
4. Define `Mutator struct` with fields: the Kubernetes object pointer, `plans []featurePlan`, `active *featurePlan`.
5. Implement `NewMutator(obj *Kind) *Mutator` — initializes and calls `beginFeature()`.
6. Implement `beginFeature()` (unexported) — appends a new `featurePlan` to `plans` and sets `active`.
7. For each edit category: implement a recording method (e.g. `EditObjectMetadata(func(*editors.ObjectMetaEditor) error)`).
8. Add convenience wrappers for the most common operations.
9. Implement `Apply() error` — iterates `plans`, for each plan applies categories in fixed order using the appropriate editors.

**Apply() ordering convention:**
- Metadata edits always run first (step 1).
- Subsequent categories follow structural order (outer → inner, e.g. spec → pod spec → containers).
- Container presence always runs before container edits within the same feature.

### Step 2: `resource.go`

1. Define `DefaultFieldApplicator(current, desired *Kind) error` — typically `*current = *desired.DeepCopy(); return nil`.
2. Define `Resource struct` with a single unexported field: `base *generic.<Category>Resource[*Kind, *Mutator]`.
3. Implement all `component.Resource` methods by delegating to `r.base`: `Identity()`, `Object()`, `Mutate()`.
4. For each lifecycle interface the category requires, implement the method by delegating to `r.base.<Method>()`.
5. Write GoDoc on `Resource` listing all implemented interfaces. Write GoDoc on each method explaining its behavior.

**No business logic belongs in resource.go.** All logic lives in handlers.go and mutator.go.

### Step 3: `builder.go`

1. Add `// Package <kind> provides...` doc comment **only if mutator.go is not the package doc file**. (Prefer putting it on mutator.go.)
2. Define `Builder struct` with a single unexported field: `base *generic.<Category>Builder[*Kind, *Mutator]`.
3. Implement `NewBuilder(obj *Kind) *Builder` — creates an `identityFunc`, calls `generic.New<Category>Builder(...)`, registers default handlers (for Workload/Task/Integration), returns `&Builder{base: ...}`.
4. Implement `WithMutation(m Mutation) *Builder` — wraps and delegates: `b.base.WithMutation(feature.Mutation[*Mutator](m))`.
5. Implement `WithCustomFieldApplicator`, `WithFieldApplicationFlavor`, `WithDataExtractor`.
   - For `WithDataExtractor`: take a value copy `func(Kind) error`, wrap to pointer: `func(obj *Kind) error { return extractor(*obj) }`.
   - For `WithFieldApplicationFlavor`: cast `FieldApplicationFlavor` to `generic.FieldApplicationFlavor[*Kind]`.
6. For Workload: implement `WithCustomConvergeStatus`, `WithCustomGraceStatus`, `WithCustomSuspendStatus`, `WithCustomSuspendMutation`, `WithCustomSuspendDeletionDecision`.
7. For Task: implement `WithCustomConvergeStatus`, `WithCustomSuspendStatus`, `WithCustomSuspendMutation`, `WithCustomSuspendDeletionDecision`.
8. For Integration: implement `WithCustomOperationalStatus`, `WithCustomSuspendStatus`, `WithCustomSuspendMutation`, `WithCustomSuspendDeletionDecision`.
9. Implement `Build() (*Resource, error)` — calls `b.base.Build()` and wraps result.

**All `WithX` methods return `*Builder` for fluent chaining.**

### Step 4: `flavors.go`

1. Define `type FieldApplicationFlavor flavors.FieldApplicationFlavor[*Kind]`.
2. Implement `PreserveCurrentLabels` and `PreserveCurrentAnnotations` as standalone functions with the exact signature `(applied, current, desired *Kind) error`, delegating to `flavors.PreserveCurrentLabels[*Kind]()(applied, current, desired)`.
3. Add kind-specific flavors as needed (e.g. `PreserveExternalEntries` for ConfigMap preserves `.data` keys, `PreserveCurrentPodTemplateLabels` for Deployment preserves pod template labels).

### Step 5: `handlers.go` (Workload, Task, Integration only)

Implement `Default*Handler` functions that are wired in by `NewBuilder`. These are exported so callers can compose them in custom handlers.

**Workload handler signatures:**
```go
func DefaultConvergingStatusHandler(op concepts.ConvergingOperation, obj *Kind) (concepts.AliveStatusWithReason, error)
func DefaultGraceStatusHandler(obj *Kind) (concepts.GraceStatusWithReason, error)
func DefaultSuspendMutationHandler(m *Mutator) error
func DefaultSuspensionStatusHandler(obj *Kind) (concepts.SuspensionStatusWithReason, error)
func DefaultDeleteOnSuspendHandler(obj *Kind) bool
```

**Task handler signatures:**
```go
func DefaultConvergingStatusHandler(op concepts.ConvergingOperation, obj *Kind) (concepts.CompletionStatusWithReason, error)
func DefaultSuspendMutationHandler(m *Mutator) error
func DefaultSuspensionStatusHandler(obj *Kind) (concepts.SuspensionStatusWithReason, error)
func DefaultDeleteOnSuspendHandler(obj *Kind) bool
```

**Integration handler signatures:**
```go
func DefaultOperationalStatusHandler(op concepts.ConvergingOperation, obj *Kind) (concepts.OperationalStatusWithReason, error)
// Suspend handlers only if the integration supports suspension
```

---

## Identity Format

Identity strings follow the pattern `<apiVersion>/<kind>/<namespace>/<name>`:

| Kind        | Identity format                             |
|-------------|---------------------------------------------|
| Deployment  | `apps/v1/Deployment/<namespace>/<name>`     |
| ConfigMap   | `v1/ConfigMap/<namespace>/<name>`           |
| StatefulSet | `apps/v1/StatefulSet/<namespace>/<name>`    |
| DaemonSet   | `apps/v1/DaemonSet/<namespace>/<name>`      |
| Job         | `batch/v1/Job/<namespace>/<name>`           |
| Service     | `v1/Service/<namespace>/<name>`             |
| Ingress     | `networking.k8s.io/v1/Ingress/<namespace>/<name>` |
| Secret      | `v1/Secret/<namespace>/<name>`              |
| ServiceAccount | `v1/ServiceAccount/<namespace>/<name>`   |
| CronJob     | `batch/v1/CronJob/<namespace>/<name>`       |

---

## Mutation System

The Mutator uses a **plan-and-apply** pattern:

1. `NewMutator(obj)` is called by the framework before each mutation pass. It calls `beginFeature()` to open the first feature scope.
2. For each enabled `Mutation`, the framework calls `beginFeature()` on the Mutator to open a new scope, then calls `Mutate(mutator)`.
3. After all mutations are recorded, the framework calls `Apply()` which iterates all feature plans and applies edits in fixed order.

`beginFeature()` is called by the framework via the `FeatureMutator` interface (which requires `beginFeature()` as an unexported method). The Mutator must implement this interface:

```go
// internal/generic/resource_workload.go
type FeatureMutator interface {
    MutatorApplier
    beginFeature()
}
```

Mutators for all categories (including Static) must implement `beginFeature()`. The framework uses it only when the Mutator implements `FeatureMutator`.

---

## Available Editors

All editors are in `pkg/mutation/editors/`.

| Editor | Constructor | Scope |
|--------|-------------|-------|
| `ObjectMetaEditor` | `editors.NewObjectMetaEditor(&obj.ObjectMeta)` | Labels, annotations on any object |
| `DeploymentSpecEditor` | `editors.NewDeploymentSpecEditor(&obj.Spec)` | Replicas, strategy, paused, etc. |
| `PodSpecEditor` | `editors.NewPodSpecEditor(&obj.Spec.Template.Spec)` | Volumes, tolerations, service account, node selectors, security context |
| `ContainerEditor` | `editors.NewContainerEditor(&container)` | Env vars, args, resources |
| `ConfigMapDataEditor` | `editors.NewConfigMapDataEditor(&obj.Data, &obj.BinaryData)` | `.data` and `.binaryData` entries |

Every editor exposes `Raw()` for direct access to the underlying Kubernetes struct.

### ContainerEditor methods
`EnsureEnvVar`, `EnsureEnvVars`, `RemoveEnvVar`, `RemoveEnvVars`, `EnsureArg`, `EnsureArgs`, `RemoveArg`, `RemoveArgs`, `SetResourceLimit`, `SetResourceRequest`, `SetResources`, `Raw`

### PodSpecEditor methods
`SetServiceAccountName`, `EnsureVolume`, `RemoveVolume`, `EnsureToleration`, `RemoveTolerations`, `EnsureNodeSelector`, `RemoveNodeSelector`, `EnsureImagePullSecret`, `RemoveImagePullSecret`, `SetPriorityClassName`, `SetHostNetwork`, `SetHostPID`, `SetHostIPC`, `SetSecurityContext`, `Raw`

### DeploymentSpecEditor methods
`SetReplicas`, `SetPaused`, `SetMinReadySeconds`, `SetRevisionHistoryLimit`, `SetProgressDeadlineSeconds`, `Raw`

### ObjectMetaEditor methods
`EnsureLabel`, `RemoveLabel`, `EnsureAnnotation`, `RemoveAnnotation`, `Raw`

### ConfigMapDataEditor methods
`Set`, `Remove`, `MergeYAML`, `SetBinary`, `RemoveBinary`, `Raw`, `RawBinary`

---

## Container Selectors

```go
selectors.AllContainers()
selectors.ContainerNamed("app")
selectors.ContainersNamed("web", "api")
selectors.ContainerAtIndex(0)
```

Selectors are only relevant for Mutators that manage pod-bearing objects (Deployment, StatefulSet, DaemonSet, Job).

---

## Testing Conventions

- Test files use `package <kind>` (same package — white-box access is allowed for tests).
- Use `testify/assert` and `testify/require`. Ginkgo/Gomega is available but not used in existing primitive tests.
- Required test coverage for every primitive:
  - `TestResource_Identity` — verifies the identity string format.
  - `TestResource_Object` — verifies a deep copy is returned.
  - `TestResource_Mutate` — verifies baseline field application on an empty current object.
  - `TestResource_Mutate_WithMutation` — verifies a mutation is applied.
  - `TestResource_Mutate_FeatureOrdering` — verifies second mutation observes changes from first.
  - `TestResource_Mutate_CustomFieldApplicator` — verifies custom applicator overrides default.
  - `TestResource_ExtractData` — verifies data extractor is called with a value copy.
  - For Workload/Task/Integration: status handler tests (default and custom handlers).
  - For Workload: suspend tests (`Suspend`, `SuspensionStatus`, `DeleteOnSuspend`).
  - Builder validation tests (`TestBuilder_Build_Validation`).
  - Mutator-level tests in `mutator_test.go` (plan-and-apply correctness, each edit method).
  - Handler-level tests in `handlers_test.go` (each default handler, all status transitions).
  - Flavor tests in `flavors_test.go`.

---

## Primitives Still to Implement

The following primitives are mentioned in the documentation or are obvious candidates based on the primitive categories. None exist yet in `pkg/primitives/`:

### Workload

#### `StatefulSet` (`pkg/primitives/statefulset/`)
- **Generic base:** `WorkloadResource[*appsv1.StatefulSet, *Mutator]`
- **Implements:** `Alive`, `Graceful`, `Suspendable`, `DataExtractable`
- **Identity:** `apps/v1/StatefulSet/<namespace>/<name>`
- **Notable implementation details:**
  - Suspension: StatefulSets support `.spec.replicas = 0` (same pattern as Deployment). Alternatively, use `.spec.updateStrategy.type = OnDelete` to pause rollouts. The simplest default is scale to zero.
  - Convergence status: check `Status.ReadyReplicas == *Spec.Replicas`. Also consider `Status.CurrentRevision == Status.UpdateRevision` for update detection.
  - Grace status: same pattern as Deployment — Degraded if ReadyReplicas > 0 but < desired; Down if ReadyReplicas == 0.
  - DefaultDeleteOnSuspendHandler: `false` (keep PVCs alive, scale to zero).
  - **Editors to expose on Mutator:** `EditObjectMetadata`, `EditStatefulSetSpec` (new editor needed, or use Raw), `EditPodTemplateMetadata`, `EditPodSpec`, `EnsureContainer`, `RemoveContainer`, `EditContainers`, `EnsureInitContainer`, `RemoveInitContainer`, `EditInitContainers`.
  - **Flavors:** `PreserveCurrentLabels`, `PreserveCurrentAnnotations`, `PreserveCurrentPodTemplateLabels`, `PreserveCurrentPodTemplateAnnotations`.
  - StatefulSet `spec.selector` and `spec.serviceName` are **immutable** after creation — the default field applicator must not touch them, or a custom applicator must be used. Consider a `PreserveCurrentSelector` flavor or a careful default applicator.

#### `DaemonSet` (`pkg/primitives/daemonset/`)
- **Generic base:** `WorkloadResource[*appsv1.DaemonSet, *Mutator]`
- **Implements:** `Alive`, `Graceful`, `Suspendable`, `DataExtractable`
- **Identity:** `apps/v1/DaemonSet/<namespace>/<name>`
- **Notable implementation details:**
  - DaemonSets have no `spec.replicas`. There is no native scale-to-zero.
  - Suspension strategy options: (a) add a node selector that matches no nodes (e.g. `kubernetes.io/os: nonexistent`), (b) delete the DaemonSet. Default `DeleteOnSuspend` should be `true` or use the node-selector approach — document which you choose.
  - Convergence status: check `Status.NumberReady == Status.DesiredNumberScheduled`. A DaemonSet with zero desired nodes is trivially healthy.
  - Grace status: Degraded if `NumberReady > 0` but `< DesiredNumberScheduled`; Down if `NumberReady == 0 && DesiredNumberScheduled > 0`.
  - **Editors to expose on Mutator:** `EditObjectMetadata`, `EditDaemonSetSpec` (new editor or Raw), `EditPodTemplateMetadata`, `EditPodSpec`, `EnsureContainer`, `RemoveContainer`, `EditContainers`, `EnsureInitContainer`, `RemoveInitContainer`, `EditInitContainers`.
  - **Flavors:** `PreserveCurrentLabels`, `PreserveCurrentAnnotations`, `PreserveCurrentPodTemplateLabels`, `PreserveCurrentPodTemplateAnnotations`.

### Task

#### `Job` (`pkg/primitives/job/`)
- **Generic base:** `TaskResource[*batchv1.Job, *Mutator]`
- **Implements:** `Completable`, `Suspendable`, `DataExtractable`
- **Identity:** `batch/v1/Job/<namespace>/<name>`
- **Notable implementation details:**
  - Jobs are largely immutable after creation. The default field applicator should be conservative — a `*current = *desired.DeepCopy()` applicator will cause Issues with immutable fields (`spec.selector`, `spec.template`). Implementers should use a custom applicator or preserve immutable fields via a flavor.
  - Convergence / completion status: `Status.Succeeded > 0` → `CompletionStatusCompleted`; `Status.Active > 0` → `CompletionStatusRunning`; `Status.Failed > 0` → `CompletionStatusFailing`; otherwise `CompletionStatusPending`.
  - Suspension: Jobs support `.spec.suspend = true` natively (Kubernetes 1.22+). `DefaultSuspendMutationHandler` sets `.spec.suspend = true`. `DefaultSuspensionStatusHandler` checks `Status.Active == 0 && Status.Succeeded == 0` (or check `Status.Conditions` for the `Suspended` type). `DefaultDeleteOnSuspendHandler`: `false`.
  - **Editors to expose on Mutator:** `EditObjectMetadata`, `EditJobSpec` (new editor or Raw), `EditPodTemplateMetadata`, `EditPodSpec`, `EnsureContainer`, `RemoveContainer`, `EditContainers`, `EnsureInitContainer`, `RemoveInitContainer`, `EditInitContainers`.
  - **Flavors:** `PreserveCurrentLabels`, `PreserveCurrentAnnotations`.

### Integration

#### `Service` (`pkg/primitives/service/`)
- **Generic base:** `IntegrationResource[*corev1.Service, *Mutator]`
- **Implements:** `Operational`, `DataExtractable`
- **Identity:** `v1/Service/<namespace>/<name>`
- **Notable implementation details:**
  - Services are usually immediately operational after creation. Operational status depends on service type:
    - `ClusterIP`: always Operational once created.
    - `LoadBalancer`: Operational when `Status.LoadBalancer.Ingress` is non-empty; Pending otherwise.
    - `NodePort`: always Operational once created.
  - The default `OperationalStatusHandler` should check `.spec.type` and act accordingly.
  - Services have an immutable `.spec.clusterIP` after creation. The default field applicator should preserve it: `current.Spec.ClusterIP = current.Spec.ClusterIP` (i.e. copy desired but restore clusterIP). Alternatively implement a `PreserveClusterIP` flavor.
  - Services also have an immutable `spec.selector` in some configurations. Consider a `PreserveCurrentSelector` flavor.
  - No suspension needed in most cases. If added: deleting the Service on suspend is the typical approach.
  - **Editors to expose on Mutator:** `EditObjectMetadata`, `EditServiceSpec` (new editor or Raw — covers ports, type, selector, etc.).
  - **Data extraction:** common use case — extract the assigned `ClusterIP` or `LoadBalancer.Ingress[0].IP` for use by other resources.
  - **Flavors:** `PreserveCurrentLabels`, `PreserveCurrentAnnotations`, `PreserveClusterIP` (kind-specific).

#### `Ingress` (`pkg/primitives/ingress/`)
- **Generic base:** `IntegrationResource[*networkingv1.Ingress, *Mutator]`
- **Implements:** `Operational`, `DataExtractable`
- **Identity:** `networking.k8s.io/v1/Ingress/<namespace>/<name>`
- **Notable implementation details:**
  - Operational when `Status.LoadBalancer.Ingress` is non-empty; Pending until an ingress controller assigns an address.
  - No suspension in the typical case.
  - **Editors to expose on Mutator:** `EditObjectMetadata`, `EditIngressSpec` (new editor or Raw — covers rules, TLS, ingressClassName).
  - **Data extraction:** extract assigned hostname or IP from `Status.LoadBalancer.Ingress`.
  - **Flavors:** `PreserveCurrentLabels`, `PreserveCurrentAnnotations`.

#### `CronJob` (`pkg/primitives/cronjob/`)
- **Generic base:** `IntegrationResource[*batchv1.CronJob, *Mutator]`
- **Implements:** `Operational`, `Suspendable`, `DataExtractable`
- **Identity:** `batch/v1/CronJob/<namespace>/<name>`
- **Notable implementation details:**
  - CronJobs are considered `Operational` when the schedule is valid and the CronJob is not suspended.
  - Suspension: CronJobs support `.spec.suspend = true` natively. `DefaultSuspendMutationHandler` sets it. `DefaultSuspensionStatusHandler` checks `.spec.suspend == true`. `DefaultDeleteOnSuspendHandler`: `false`.
  - Convergence / operational status: Operational if `.spec.suspend != true`; use `Status.LastScheduleTime` or `Status.Active` as additional signals.
  - **Editors to expose on Mutator:** `EditObjectMetadata`, `EditCronJobSpec` (new editor or Raw — covers schedule, concurrencyPolicy, startingDeadlineSeconds, etc.), `EditJobTemplateSpec`, `EditPodTemplateMetadata`, `EditPodSpec`, `EnsureContainer`, `RemoveContainer`, `EditContainers`.
  - **Flavors:** `PreserveCurrentLabels`, `PreserveCurrentAnnotations`.

### Static

#### `Secret` (`pkg/primitives/secret/`)
- **Generic base:** `StaticResource[*corev1.Secret, *Mutator]`
- **Implements:** `DataExtractable`
- **Identity:** `v1/Secret/<namespace>/<name>`
- **Notable implementation details:**
  - Structurally identical to ConfigMap. The `.data` field is `map[string][]byte` (binary). No `MergeYAML` — binary data is not YAML-mergeable.
  - A `SecretDataEditor` likely needs to be added to `pkg/mutation/editors/`. It should expose `Set(key string, value []byte)`, `Remove(key string)`, and `Raw() map[string][]byte`.
  - The `.stringData` field is write-only in the Kubernetes API — the API server converts it to `.data`. The default field applicator should use `.data` only. Do not expose `.stringData` mutation unless specifically needed.
  - **Flavors:** `PreserveCurrentLabels`, `PreserveCurrentAnnotations`, `PreserveExternalEntries` (preserve `.data` keys added externally, analogous to ConfigMap's flavor).
  - **Security consideration:** Data extractors on Secrets handle sensitive values. GoDoc should note that extracted values should be treated as sensitive.

#### `ServiceAccount` (`pkg/primitives/serviceaccount/`)
- **Generic base:** `StaticResource[*corev1.ServiceAccount, *Mutator]`
- **Implements:** _(no additional lifecycle interfaces typically)_
- **Identity:** `v1/ServiceAccount/<namespace>/<name>`
- **Notable implementation details:**
  - Very simple — mostly just metadata. The Mutator needs only `EditObjectMetadata`.
  - `imagePullSecrets` and `secrets` fields may be managed by other controllers (token controller). Consider a `PreserveCurrentSecrets` flavor.
  - **Flavors:** `PreserveCurrentLabels`, `PreserveCurrentAnnotations`.

---

## Shared Files to Modify (Handle Manually)

The following files are shared infrastructure. **Multiple parallel sessions must not both modify these.** Handle them in a single consolidation step after all primitive packages are implemented.

| File | What to add |
|------|-------------|
| `docs/primitives.md` | Add each new primitive to the "Built-in Primitives" table at the bottom |
| `pkg/mutation/editors/` | New editor types required by new primitives (e.g. `SecretDataEditor`, or kind-specific spec editors). Each editor lives in its own file: `secretdata.go`, `statefulsetspec.go`, etc. Add it to the "Mutation Editors" table in `docs/primitives.md`. |
| `examples/` | Each primitive should have an `examples/<kind>-primitive/` directory mirroring the deployment-primitive and configmap-primitive examples |
| `go.mod` / `go.sum` | If any new external dependencies are added (unlikely — all Kubernetes kinds are already in `k8s.io/api`) |

---

## New Editor Checklist

If a new editor is needed (e.g. `StatefulSetSpecEditor`, `ServiceSpecEditor`), it must:

1. Live in `pkg/mutation/editors/<kindspec>.go`.
2. Have a `New<Kind>SpecEditor(spec *apiKind.Spec) *<Kind>SpecEditor` constructor.
3. Expose `Raw() *apiKind.Spec`.
4. Expose typed setter methods for commonly-used fields.
5. Have a corresponding `<kindspec>_test.go`.
6. Be documented in `docs/primitives.md` in the "Mutation Editors" table.

---

## GoDoc Checklist

Every exported symbol in a primitive package must have a GoDoc comment. Minimum required:

- **Package**: one-line summary on the package declaration in the package doc file.
- **`Mutation`**: explain it is the public alias for `feature.Mutation[*Mutator]`.
- **`Mutator`**: describe plan-and-apply, feature boundaries, `Apply()` ordering.
- **`NewMutator`**: describe its role (used within feature mutations).
- **Each edit method on `Mutator`**: describe planning, execution order, nil-safety.
- **`Apply()`**: document the full execution order with numbered steps.
- **`DefaultFieldApplicator`**: describe what "replace current with desired" means.
- **`Resource`**: list all implemented interfaces.
- **Each method on `Resource`**: describe framework integration, default behavior, override mechanism.
- **`Builder`**: one-line summary.
- **`NewBuilder`**: describe what the object represents (desired base state) and validation requirements.
- **Each `WithX` method on `Builder`**: describe when to use it and the default it overrides.
- **`Build()`**: describe what is validated.
- **`FieldApplicationFlavor`**: describe when flavors run.
- **Each flavor function**: describe what it preserves, when it is useful.
- **Each `Default*Handler`**: describe the logic, which builder method it backs, and note it can be composed in custom handlers.