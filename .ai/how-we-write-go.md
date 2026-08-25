---
name: how-we-write-go
description: Use when writing, reviewing, or modifying Go code in this repository, including tests and examples.
---

# How We Write Go

This is a published Go library. Every exported name, every doc comment and every example is a contract that operators
outside this repository build against. The conventions below exist so that contract reads the same everywhere.

## Doc comments

A godoc comment is a contract. A caller must be able to understand the behavior, preconditions, and outcome of using a
function, type, or constant without reading its implementation. An inaccurate or misleading godoc is worse than none —
it produces incorrect mental models and silent bugs. Accuracy is non-negotiable for maintainability.

**When you touch code that has an inaccurate or unclear doc comment, fix it.** Do not leave it as-is just because it was
there before. First ask: is the comment wrong, or is the code wrong? Reconcile accordingly — correct the comment to
match the code, or correct the code to match the stated contract, whichever is right.

**Exported identifiers** must have a doc comment. Start with the identifier name. Write as much as clearly and
unambiguously describes the contract — no more, no less. For a simple function or constant, one sentence usually
suffices; for a type or interface with non-obvious semantics or subtle constraints, a short paragraph is appropriate.

```go
// ConvergingOperationNone indicates that the apply left the existing resource
// unchanged, or that the resource was only read (read-only resources).
ConvergingOperationNone ConvergingOperation = "None"

// Enabled reports whether the constraint is satisfied for the given version string.
func (g *VersionGate) Enabled() (bool, error) { ... }

// Gate is an optional gate for a Mutation or resource.
// If Enabled returns false, the associated mutation is not applied,
// or the associated resource is marked for deletion.
type Gate interface { ... }
```

**Internal identifiers** get no comment unless the behavior would genuinely surprise a reader. Keep it to what a reader
actually needs — not a summary of the code below it.

```go
// returns the kind for an unstructured object, which carries no Go type name
func typeName(obj any) string { ... }
```

**A godoc gives the contract, not the algorithm.** Preconditions, the meaning of the result, what the caller must not
assume. When you start a sentence about how the function computes its answer, stop: the caller does not need it, and it
goes stale the first time the body changes.

**A godoc is sized by the caller's decision.** It holds what someone needs to call the function correctly and to use
what it gives back: the preconditions, the meaning of the result, and the trap that will bite them. A fact that does not
change what the caller writes is not part of the contract, however true it is and however hard it was to learn.

**Moving a fact out of a godoc is not deleting it.** Why a piece of code exists — the case it handles, what breaks
without it — earns a short comment beside that code, where a reader meets it with the code in view, and it earns one
even when a caller never needs to know. A constraint that bites at one call site goes at that call site for the same
reason. The godoc keeps what a caller cannot see from outside at all; the pull request keeps the comparison, the options
weighed and why this one won. The same fact in the godoc and again at the line it constrains is one copy too many, and
the godoc copy is the one that rots, because it sits furthest from the code that would contradict it.

**When the godoc is longer than the function body, name the caller decision each paragraph serves.** Move the paragraphs
that serve none to the code they explain, and delete the ones that explain nothing. Twenty lines of prose over a
one-line body is the clearest case: nobody needed that much to call it.

Rationale in a godoc costs more than the space it takes. It reads as contract, so the next reader treats it as a promise
the code has to keep, and the next change argues with the paragraph instead of the code.

**A review finding is not a reason to write a paragraph.** When a review turns up a case the code missed, the fix is the
code. Write the comment only if the next reader would be caught by the same thing and could not deduce it from what is
in front of them — not to show the case was considered, and not to record that the round happened.

**The test for a bad comment:** could a code generator produce it by prepending a verb to the identifier name? If yes,
it carries no information beyond the name itself — delete or rewrite it.

- `// Identity returns the identity` → generated noise; delete
- `// Identity returns a unique identifier for the resource in the format <apiVersion>/<kind>/<name>.` → adds
  information; keep

**Never:**

- Restate what the name already says (apply the code-generator test above)
- Leak implementation context that will rot (`// Typically this will be a *appsv1.Deployment`)
- Add temporal or task context (`// In production this would...`, `// Added for the X flow`)
- Pad a comment to look thorough — every sentence must earn its place

## Inline comments

**What a comment holds.** A comment carries what the code cannot: a constraint from outside this file, an invariant a
reader cannot see from here, the reason a plainer version does not work. Write that fact first, then why it forces this
code.

```go
// Compile-time guarantee that *Mutator satisfies the shared workload editing
// surface. If a future change renames or removes a shared method, this breaks
// the build here instead of drifting silently in downstream consumers.
var _ primitives.WorkloadMutator = (*Mutator)(nil)
```

If you cannot write that first fact without paraphrasing the lines below it, there is no comment to write.

**A comment that restates the code becomes a second spec, and it drifts.** The next reader has two accounts of one
behavior and no way to tell which is current. Nothing marks the prose as the weaker source: to a reader skimming, and to
a model that reads comment and code as one stream, a stale sentence looks exactly like a statement of intent. What
follows is the argument moving off the code and onto the prose — the review debates the sentence, the fix edits the
sentence, and the behavior stays wrong. Every comment is a claim you have to keep true for as long as the code lives.
Write only the claims worth maintaining.

**Density is a signal.** When most blocks carry a comment, the comments say nothing and the one that matters is buried
among them. If a block needs prose to be followed, a better name or a smaller function comes first. The comment is the
fallback, not the fix.

**Never narrate what the code does:**

```go
// BAD — narrates WHAT
// Build the label set that will be applied to the Deployment.
labels := map[string]string{ ... }

// BAD — AI slop
// In production code this would go through the event recorder.
```

If removing the comment would not confuse a reader six months from now, delete it.

**Red flags in your own diff:**

- A comment and the line under it say the same thing in two languages.
- You wrote a comment to explain a name you could have fixed.
- The comment describes the change you are making rather than the code that is there.
- You are editing a comment to answer a review point instead of editing the code.
- A godoc grew a paragraph about how the body works.
- The godoc is longer than the function body.
- The same fact appears twice: once in the godoc, once at the line it constrains.
- You deleted a hard-won fact instead of moving it beside the code it explains.
- The comment exists because a review asked for the change, not because the next reader will need it.

## A stated behavior is a claim, not evidence

A godoc, an inline comment, a page under `docs/` and an example under `examples/` are claims someone made about the code
at the time they wrote it. A failing test, a stuck reconcile or a bug report is a measurement. When the two disagree,
the measurement settles what the code does — and the design question is still open: which behavior should the framework
have? A statement can be an accurate description of a wrong decision.

1. Read the code to find what it does now. Do not take the statement as the answer.
2. Ask whether the stated behavior is the one the operator author wants. The observed problem is evidence about that,
   and a documented contract is not a reason to keep a behavior that produces it.
3. Change whichever is wrong — the code, the statement, or both.

**A statement your change falsified is part of your change.** When your fix makes a godoc, a comment, a `docs/` page, a
`README.md` section or an example wrong, correct it in the same change and name the change in the pull request body. A
contradiction between code and prose is never shipped and never deferred. `plugin/skills/*/references/` holds synced
copies of `docs/`, so a `docs/` edit is finished by `make sync-plugin`, not by the edit alone.

| Rationalization                                              | Reality                                                                                                                                          |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| "The doc update is out of scope, this task was the code fix" | The doc became wrong when your code changed. Correcting it is the same task.                                                                     |
| "I will note it as a follow-up"                              | A follow-up leaves a false statement in `main` for everyone who reads it first. File follow-ups for work you did not do, not for damage you did. |
| "The godoc states the contract, so the fix must preserve it" | A contract is a decision, and a decision is revisable. If the reported problem shows it is the wrong one, change it and reconcile the prose.     |
| "I only touched one package"                                 | Find the statement wherever it lives: godoc, `docs/`, `README.md`, the example that demonstrates it, the synced plugin reference.                |
| "The doc is still true for the common case"                  | Partly true reads as fully true. State the behavior the code now has.                                                                            |

## Whitespace rhythm

Each logical step gets its own block, separated by a blank line. The only exception: a call and its immediate error
handler stay together — the error check is the direct continuation of the call, not a new step.

```go
// Each guard checks a different input — separate concerns, separate blocks
if name == "" {
    return errors.New("deployment: name must not be empty")
}

if obj == nil {
    return errors.New("deployment: object must not be nil")
}

// Reading: call and error handler are one unit
current, err := resource.Object()
if err != nil {
    return fmt.Errorf("reading object for resource %s: %w", resource.Identity(), err)
}

// Mutating is a separate step from reading
if err := resource.Mutate(current); err != nil {
    return fmt.Errorf("mutating resource %s: %w", resource.Identity(), err)
}

// Applying is a separate step from mutating
if err := rec.Client.Patch(ctx, current, client.Apply, fieldOwner); err != nil {
    return fmt.Errorf("applying resource %s: %w", resource.Identity(), err)
}
```

The anti-pattern to avoid is inserting blank lines _within_ a logical unit — e.g., between a call and its error handler,
or between two lines that together produce one value.

## Line breaking long calls

Nothing in `make fmt` reflows Go source: `gofmt` and `goimports` normalise indentation and imports, and leave your line
breaks exactly where you put them. The soft limit is 120 characters, the same width prettier applies to Markdown here,
and holding it is your job rather than a tool's.

When a call or a signature does not fit, break after the opening parenthesis and put the closing parenthesis on its own
line. Keep the arguments together on one continuation line when they fit; give each argument its own line when they do
not.

```go
// GOOD — one continuation line, closing paren on its own line
func newConvergingStatusCondition(
    ctx context.Context, owner OperatorCRD, results reconcileResults, gracePeriod time.Duration, previousCondition Condition,
) Condition {

// GOOD — a single long parameter gets the line to itself
func (b *Builder) WithCustomOperationalStatus(
    handler func(concepts.ConvergingOperation, *corev1.PersistentVolumeClaim) (concepts.OperationalStatusWithReason, error),
) *Builder {
```

Both of those lines still run past 120 characters, and that is the point of a soft limit: when the parameter list or a
single type expression cannot be broken any further, let it run rather than inventing a wrap that reads worse. The limit
is there to stop you cramming, not to be satisfied at any cost.

Never use the mixed form, where the first argument sits on the paren line and a later one starts on another line. It
reads as two different shapes at once and every later edit has to pick a side.

```go
// BAD — crammed onto one line
recorder.Eventf(obj, nil, corev1.EventTypeNormal, reasonApplied, actionApply, "resource %q applied", identity)

// BAD — mixed form: some args on the paren line, some not
recorder.Eventf(obj, nil, corev1.EventTypeNormal,
    reasonApplied, actionApply, "resource %q applied", identity)

// GOOD — one argument per line
recorder.Eventf(
    obj,
    nil,
    corev1.EventTypeNormal,
    reasonApplied,
    actionApply,
    "resource %q applied",
    identity,
)
```

The same rule applies to struct literals:

```go
// BAD
return &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels}}

// GOOD
return &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
        Name:      name,
        Namespace: namespace,
        Labels:    labels,
    },
}
```

## Code bloat

Every line of code is a liability: it has to be read, understood, tested, and maintained. Before writing anything, ask
whether it needs to exist at all.

**Use builtins and standard library functions.** Don't reimplement what already exists.

```go
// BAD — manual join
result := ""
for i, s := range items {
    if i > 0 {
        result += ", "
    }
    result += s
}

// GOOD
result := strings.Join(items, ", ")
```

**Grep before you write.** If you are about to write a helper, check whether one already exists in the repo. In a
library with twenty primitive packages that all wrap the same generic base, the second copy of a helper is not a
convenience — it is a second thing to keep in step. A shared helper belongs in `pkg/generic`, `pkg/primitives` or
`pkg/component/concepts`, not duplicated per kind.

**Prefer fewer, simpler constructs.** This applies at every level — conditions, functions, entire code blocks.

Multiple conditions with the same outcome belong in one guard:

```go
// BAD
if a == nil {
    return ErrInvalidInput
}
if b == nil {
    return ErrInvalidInput
}

// GOOD
if a == nil || b == nil {
    return ErrInvalidInput
}
```

A function that wraps a single call and adds nothing is not a function — it is noise:

```go
// BAD — this is just obj.Name with extra steps
func getDeploymentName(obj *appsv1.Deployment) string {
    return obj.Name
}

// GOOD — use obj.Name directly at the call site
```

Code blocks left behind after a refactor often do nothing anymore. When you change code, scan the surrounding context:
does any adjacent code now duplicate what you just added, or call something that no longer exists in meaningful form?

## Layers and side effects

The framework is a layered system: a builder configures, a resource carries identity and mutation, a mutator edits an
object in memory, and the component owns the reconcile and every call to the API server. Each layer has a defined
responsibility; mixing them creates invisible coupling and makes bugs hard to trace.

A function that does more than its name and signature suggest is impure: it produces side effects the caller cannot see,
cannot control, and cannot test in isolation. A mutator that also reads from the API server does two things while
advertising one, and it drops golden tests — which run without a cluster — on the floor.

```go
// BAD — mutation reaches out to the cluster; the caller cannot see or control the read
func (m *Mutator) SetImage(ctx context.Context, c client.Client) error {
    var cfg corev1.ConfigMap
    if err := c.Get(ctx, key, &cfg); err != nil {
        return err
    }
    m.current.Spec.Template.Spec.Containers[0].Image = cfg.Data["image"]
    return nil
}

// GOOD — the mutation is pure: values in, object edited in memory, nothing else
func (m *Mutator) SetImage(image string) {
    ...
}

// The reconcile layer does the read and passes the value in.
```

**Signal mutation in the name or doc comment.** A function that modifies a passed object must make that obvious — either
through a verb like `set`, `apply`, `mutate`, or an explicit doc comment. A reader should not have to trace into the
body to discover that the object was changed.

```go
// clear from the name
func applyContainerEdits(containers []corev1.Container, edits []containerEdit) error { ... }

// or clear from the doc comment when the name alone isn't enough
// configureProbes sets liveness and readiness probes on the container spec
// according to the component's health check settings.
func configureProbes(container *corev1.Container, cfg HealthConfig) { ... }
```

**Keep API access at the edge.** `Get`, `Patch`, `Create` and `Delete` belong to the reconcile path in `pkg/component`
and to the CLI in `cmd/ocf`. Builders, mutators, editors, selectors, gates and status handlers take values and return
values. Keeping them free of I/O is what makes them testable with a golden snapshot instead of envtest.

## Typed string constants

Any string that crosses a package boundary — a status value, an operation, a reason, a condition type, a label or
annotation key — is declared as a constant of a defined type, not written inline. The defined type is what lets the
compiler catch a typo and what lets a `switch` be exhaustive.

```go
// BAD — bare strings; nothing stops a caller passing "created"
func applyOperationAction(op string) string {
    if op == "Created" {
        return "Create"
    }
    ...
}

// GOOD — defined type, named constants, exhaustive switch
type ConvergingOperation string

const (
    ConvergingOperationCreated ConvergingOperation = "Created"
    ConvergingOperationUpdated ConvergingOperation = "Updated"
    ConvergingOperationNone    ConvergingOperation = "None"
)
```

For a string an operator author will see or match on, the constant is public API: name it once, document it, and change
it only as a deliberate breaking change.

## Error wrapping

Always wrap with `%w` to preserve the chain. Include enough context to identify the call site without a stack trace:

```go
// BAD
return err
return fmt.Errorf("failed: %v", err)

// GOOD
return fmt.Errorf("applying resource %s in component %q: %w", resource.Identity(), name, err)
```

The prefix should read as a stack of calls: `"outer: inner: leaf error"`. Do not repeat what the wrapped error already
says; add the identity the wrapped error could not know.

An exported function that can meaningfully fail returns an `error`. A library that swallows a failure leaves the
operator author with a resource that never converges and no way to find out why.

## Exported vs internal: how much context to give

| Identity                | Exported? | Doc comment?       | Detail level                                             |
| ----------------------- | --------- | ------------------ | -------------------------------------------------------- |
| Package-level const/var | yes       | required           | name-first; as long as the contract needs                |
| Exported type           | yes       | required           | what it represents, not its fields                       |
| Exported method/func    | yes       | required           | what it does, not how; length matches complexity         |
| Interface               | yes       | required           | what the implementor promises (not typical implementors) |
| Internal func/method    | no        | only if surprising | WHY, not WHAT                                            |
| Internal type/const     | no        | almost never       |                                                          |

Every package has a package comment on exactly one file — `revive`'s `package-comments` rule enforces it, and it is the
first thing a reader sees on pkg.go.dev.

## Optional configuration

Optional behavior is expressed with builder methods and functional options, not with wide exported config structs. This
keeps the zero value valid, lets a new option ship without breaking a caller, and puts the documentation on the option
rather than on a field buried in a struct.

```go
// The shape used throughout the framework
comp, err := component.NewComponentBuilder().
    WithName("database").
    WithConditionType("DatabaseReady").
    WithResource(cmResource, component.ReadOnly()).
    Build()
```

Reach for a pointer only when the zero value is a legitimate value that must stay distinguishable from "not configured".
`*bool` is the clearest case: a bare `bool` cannot carry three states, and `false` is silently dropped by `omitempty`
when the type is serialised. Everywhere else prefer the value type — a required field, a safe zero default, or a type
that is already a reference (slice, map, interface) gains nothing from a pointer and costs the caller a dereference.

## Naming

**Use the shortest name that is unambiguous in its scope.** A name should say what a thing is, not repeat where it came
from or what type it is. Type information is already in the declaration; suffixing it into the name is noise.

```go
// BAD — suffixes add no information
deploymentObj   // it's a *appsv1.Deployment; Obj adds nothing
mutatorStruct   // it's a *Mutator; Struct adds nothing
optionsMap      // the type says map

// GOOD
deployment
mutator
options
```

**No stuttering.** If the package is `deployment`, the primary type does not need `Deployment` in its name at the call
site — `deployment.Resource` and `deployment.Builder` read fine, `deployment.DeploymentResource` repeats itself. This
matters more here than in a private codebase: the package path is part of every call site an operator author writes.

**Acronyms are all-caps.** `HTTPClient`, `userID`, `parseURL` — not `HttpClient`, `userId`, `parseUrl`. `GVK`, `PVC` and
`HPA` follow the same rule.

**Short names are appropriate in short scopes.** `i`, `v`, `k`, `err` are idiomatic in loops and brief blocks. Use more
descriptive names at package scope or in functions where the variable lives far from its declaration.

**Receiver names** are one or two letters, consistent across all methods of a type, and never `self` or `this`. `revive`
flags the inconsistency, but it is worth getting right the first time.

```go
// BAD
func (self *Builder) Build() (*Resource, error) { ... }
func (b *Builder) WithMutation(ms ...Mutation) *Builder { ... } // inconsistent with above

// GOOD — pick one and use it everywhere on the type
func (b *Builder) Build() (*Resource, error) { ... }
func (b *Builder) WithMutation(ms ...Mutation) *Builder { ... }
```

## Context propagation

`context.Context` is always the first parameter of any function that does I/O or calls into the Kubernetes API. It is
always named `ctx`. It is never stored in a struct — a stored context outlives the request it belongs to, bypasses
cancellation, and makes the function's dependencies invisible. `revive`'s `context-as-argument` rule enforces the
position; the rest is on you.

```go
// BAD — context stored in struct
type Component struct {
    ctx    context.Context
    client client.Client
}

// GOOD — context flows through the call chain as an explicit parameter
func (c *Component) Reconcile(ctx context.Context, rec ReconcileContext) error {
    results, err := c.reconcileResources(ctx, rec)
    if err != nil {
        return fmt.Errorf("reconciling resources: %w", err)
    }
    ...
}
```

Do not create a new `context.Background()` mid-call — that discards the caller's deadline and cancellation signal. A
library that does this takes the operator author's shutdown handling away from them.

## Interface design

Define interfaces in the package that consumes them, not the package that implements them. Go's implicit satisfaction
means implementations do not need to know about the interface; the consumer declares exactly the contract it needs.
`pkg/component/concepts` is the consumer side of this: it declares what the reconcile needs from a resource, and each
primitive satisfies whichever of those it can.

Keep interfaces as small as feasible — declare only the methods the consumer actually needs. An interface that requires
more than the consumer uses forces implementors to satisfy a broader contract than necessary, and usually signals that
the abstraction is wrong. In a library, it also forces that burden on every operator author who writes a custom wrapper.

Accept interfaces, return concrete structs. Callers can always widen a concrete return to an interface; narrowing in the
other direction requires a type assertion.

Do not create an interface speculatively. Create one when you have two real implementations or a genuine need to test
against a substitute. An interface with one implementation that never changes is indirection for its own sake.

```go
// BAD — one fat interface every wrapper must satisfy in full
type Resource interface {
    Identity() string
    Mutate(current client.Object) error
    ConvergingStatus(op ConvergingOperation) (AliveStatusWithReason, error)
    SuspensionStatus() (SuspensionStatusWithReason, error)
    GuardStatus() (GuardStatusWithReason, error)
}

// GOOD — one small interface per capability; a wrapper implements only what it supports
type Alive interface {
    ConvergingStatus(op ConvergingOperation) (AliveStatusWithReason, error)
}

type Guardable interface {
    GuardStatus() (GuardStatusWithReason, error)
}
```

**Do not wrap existing controller-runtime interfaces.** This guideline is about creating new interfaces where none
exists. When controller-runtime or client-go already provides an interface that covers what you need — `client.Object`,
`client.Client`, `events.EventRecorder` — use it directly. Do not re-declare methods from it into a new interface.
Re-declaring `GetName() string` and `GetNamespace() string` manually produces an interface that satisfies your contract
but not the framework's, causing type errors when the value is passed to any controller-runtime API (`recorder.Eventf`,
`client.Get`, etc.) that takes `client.Object` or `runtime.Object`.

```go
// BAD — re-declares methods already on client.Object; won't satisfy recorder.Eventf
type Owner interface {
    GetName() string
    GetNamespace() string
}

// GOOD — embed client.Object and declare only what it does not already give you
type OperatorCRD interface {
    client.Object

    GetStatusConditions() *[]metav1.Condition
    GetKind() string
}
```

**A compile-time assertion is cheaper than a broken downstream build.** When a type must keep satisfying an interface it
does not name, assert it:

```go
var _ primitives.WorkloadMutator = (*Mutator)(nil)
```

## Panic vs error

Never call `panic` on any path an operator's reconcile can reach. A panic in a controller goroutine crashes the manager
process and takes every other controller in that operator down with it. This is a library: the process it kills is not
ours. All runtime errors are returned as `error`.

`panic` is appropriate only for a programmer error caught at construction time — a misuse that is structurally
impossible to recover from and that shows up the first time the code runs:

```go
// OK — panics when an editor is constructed against a nil target; caught on the first run
func NewPolicyRulesEditor(rules *[]rbacv1.PolicyRule) *PolicyRulesEditor {
    if rules == nil {
        panic("NewPolicyRulesEditor: rules must be a non-nil pointer")
    }
    ...
}
```

If you find yourself wanting to panic at runtime because "this should never happen", return an error with enough context
to diagnose it instead. Builders collect their errors and return them from `Build`; follow that pattern rather than
failing loudly halfway through configuration.

## Where to put code

File and package organization follows concern, not type. When a group of related functions grows beyond what fits
naturally in one file, give it its own file named after the concern it serves — not after the type it operates on.

**`pkg/` is public API; `internal/` is not.** Everything an operator author imports lives under `pkg/`, and moving a
symbol there is a commitment. Everything that supports the repository itself — the CLI's scaffolding templates, the
observability rendering, scope resolution — lives under `internal/`, where it can change without a release note. Test
helpers that operator authors use are public API too, which is why `pkg/testing/` is not `internal/testing/`.

```
pkg/component/              ← builder, reconcile, conditions, status model, data cells
pkg/component/concepts/     ← lifecycle interfaces and status vocabulary; pure types, no I/O
pkg/primitives/<kind>/      ← one package per Kubernetes kind
pkg/primitives/             ← WorkloadMutator: the editing surface shared by pod workloads
pkg/mutation/editors/       ← reusable editors
pkg/mutation/selectors/     ← container selectors
pkg/generic/                ← generic bases the per-kind packages instantiate
pkg/feature/                ← gates and Mutation[T]
pkg/recording/, pkg/metrics/← event recording and metrics
pkg/testing/                ← golden, goldengen, integration helpers
internal/                   ← scaffold templates, observability rendering, scope; no API promise
cmd/ocf/                    ← CLI wiring only
```

**A primitive package has a fixed set of file roles.** Follow it for every kind, including a new one, so that a reader
who knows one primitive knows them all:

```
pkg/primitives/deployment/
  builder.go    ← package comment, Builder, NewBuilder, the With* methods, Build
  resource.go   ← Resource, its identity, and the concept interfaces it satisfies
  mutator.go    ← Mutation type alias, Mutator, the editing surface
  handlers.go   ← DefaultConvergingStatusHandler and the other default status handlers
```

`builder.go`, `resource.go` and `mutator.go` exist for every kind; `handlers.go` exists for every kind with an
observable status. Anything beyond those four gets a file named after the concern, never after the type:
`configmap/hash.go`, not `configmap/configmap_extra.go`.

**When a concern earns its own file, split it out — and make the split complete.** If a coherent set of functions has
grown distinct enough to justify a separate file, move it to that file and pull in any related code that already exists
elsewhere in the package and belongs to the same concern. A partial split — some of the concern in the new file, the
rest left behind — is worse than no split at all because it scatters the concern across files without making either file
coherent.

**Behavior that more than one primitive needs is not a primitive's code.** Duplicating a status handler or an edit
helper across kinds is how twenty packages drift apart. Lift it to `pkg/generic`, `pkg/primitives` or
`pkg/component/concepts` and instantiate it per kind.

**Every public change lands with its example.** `examples/` is compiled and run by `make all`, and each directory
demonstrates one concept. A new capability that no example exercises is a capability nobody will find.

## Order within a file

A file is read top to bottom, once, by someone who has never seen it. Order it so every line is answerable by what came
above it. A reader who meets a helper before the code that needs it carries it with no reason to; a reader who meets the
caller first already knows what the helper is for by the time it arrives.

**A file has this order:**

1. Shared declarations: the `const`, `type` and `var` that another file reads, each with its doc comment. Declarations
   only — a function never belongs in this group, however many files call it.
2. The entry point: the exported function the file exists for. It is the first function in the file.
3. Every other function, in the order the entry point reaches them — its own calls first, then theirs. An exported
   function that other packages call is placed by this rule too. Being exported does not lift a function above the entry
   point.
4. A helper with one caller sits directly under that caller, never at the end of the file.

```go
// pkg/primitives/<kind>/builder.go
type Builder struct{ ... }                               // 1. shared surface, with its doc comment

func NewBuilder(deployment *appsv1.Deployment) *Builder  // 2. what the file is for
func (b *Builder) WithMutation(ms ...Mutation) *Builder  // 3. reached from the builder surface
func (b *Builder) WithCustomGraceStatus(...) *Builder    // 3. same
func (b *Builder) Build() (*Resource, error)             // 3. the terminal call
func applyDefaults(deployment *appsv1.Deployment)        // 4. Build is its only caller
```

**A type that another file reads is not a local helper.** Declare it at the top with the other shared declarations and
give it a doc comment, whatever its visibility — an unexported type is still shared surface for the rest of the package.
A type declared at line 300 because that is where the author needed it costs every later reader a search. A type used by
one function in one file belongs next to that function.

**Place new code by the file's order, not where you were working and not at the end.** Appending to the bottom is how a
file loses its order one change at a time.

**Reordering is part of the change.** If your new function belongs between two existing ones, move it there. Moving a
function you already touched is free; a whole-file reshuffle is not, so leave that to its own commit.

When no order makes the file read straight through, the file holds more than one concern. Split it by concern, as above.

**Red flags in your own diff:**

- The first function in the file is not the one the file exists for.
- You added a function at the end of the file and its caller is somewhere above it.
- A constructor or a resolver sits above the code that consumes it because it is exported.

## Common mistakes

| Mistake                                                            | Fix                                                                                  |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------ |
| Comment restates the name                                          | Delete the comment                                                                   |
| "Typically this will be a X" in an interface doc                   | Delete; if you must constrain, use a constraint type                                 |
| Multi-sentence doc where one sentence would be complete            | Trim to what the contract actually requires                                          |
| `// In production code this would...`                              | Delete; write the real code or a `// TODO(#NNN)`                                     |
| Inline `const reason = "..."` in a function                        | Promote to a package-level constant of a defined type                                |
| Bare string where a status or operation type exists                | Use the defined type from `pkg/component/concepts`                                   |
| `fmt.Sprintf("%s-%s", a, b)`                                       | `a + "-" + b`                                                                        |
| Comment restates the line under it                                 | Delete it; if the line needs prose, rename or split instead                          |
| Godoc explains how the body computes the answer                    | Cut to the contract: preconditions, result, what the caller must not assume          |
| Godoc repeats a fact already commented at the line it constrains   | Delete the copy in the godoc; the one next to the code is the one that stays true    |
| Paragraph added because a review found a missing case              | Ship the fix; write the comment only if the next reader would be caught the same way |
| Godoc is longer than the function body                             | Name the caller decision each paragraph serves; delete the rest                      |
| Mutator, editor or gate that reads from the API server             | Pass the value in; keep I/O in the reconcile path                                    |
| Status handler copied into a second primitive package              | Lift it to `pkg/generic` or `pkg/component/concepts` and instantiate it              |
| Helper placed above the entry point of the file                    | Move it below its caller                                                             |
| Shared type declared where the author first needed it              | Move it to the top of the file with a doc comment                                    |
| New code appended to the bottom of the file                        | Place it by the file's order, under its caller                                       |
| Fix ships with a godoc, `docs/` page or example it just made wrong | Correct the statement in the same change, and `make sync-plugin` if `docs/` changed  |
