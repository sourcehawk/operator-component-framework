// Package component provides the core framework for managing Kubernetes resources as logical components.
package component

import (
	"errors"
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)

// Builder implements the fluent API for constructing and validating a Component.
// It ensures that a component is configured with consistent rules before it is
// used in a reconciliation loop.
type Builder struct {
	component   *Component
	buildErrors []error
	// resourceErrors maps a resource Identity() to a pending resolution error.
	// A successful re-registration of the same identity clears the entry so
	// the earlier error is not surfaced by Build.
	resourceErrors map[string]error

	nameSupplied          bool
	conditionTypeSupplied bool
	featureGateSupplied   bool
}

// NewComponentBuilder initializes a new Builder for creating a Component.
//
// A Component manages a single condition on an owning object (the OperatorCRD) and aggregates
// the lifecycle, readiness, and suspension state of all registered resources.
func NewComponentBuilder() *Builder {
	return &Builder{
		component: &Component{
			name:           "",
			suspended:      false,
			conditionType:  "",
			gracePeriod:    time.Duration(0),
			resourceLookup: make(map[string]Resource),
		},
		resourceErrors: make(map[string]error),
	}
}

// Build finalizes the component configuration and validates all settings and resources added.
//
// If any validation errors occurred during the fluent configuration (e.g., duplicate resources
// or invalid grace periods), Build returns a single aggregated error containing all failures
// using errors.Join.
//
// Returns:
//   - *Component: The fully configured and validated component instance on success.
//   - error: An aggregated error containing all validation failures, or nil if successful.
func (b *Builder) Build() (*Component, error) {
	if !b.nameSupplied {
		b.buildErrors = append(b.buildErrors, errors.New(
			"component name must be supplied using WithName",
		))
	}

	if !b.conditionTypeSupplied {
		b.buildErrors = append(b.buildErrors, errors.New(
			"condition type must be supplied using WithConditionType",
		))
	}

	for _, err := range b.resourceErrors {
		b.buildErrors = append(b.buildErrors, err)
	}

	if len(b.buildErrors) > 0 {
		return nil, errors.Join(b.buildErrors...)
	}
	return b.component, nil
}

// WithName sets the name of the component for logging and status identification.
//
// The name is used as the field name in the aggregation of status conditions
// and must be unique within the owning reconciler.
//
// Parameters:
//   - name: A non-empty string identifying the component.
//
// If the name is empty, a validation error is recorded and will be returned by Build().
func (b *Builder) WithName(name string) *Builder {
	b.nameSupplied = true

	if name == "" {
		b.buildErrors = append(b.buildErrors, errors.New("component name cannot be empty"))
	}

	b.component.name = name
	return b
}

// WithConditionType sets the Kubernetes condition type associated with this component.
//
// This condition type will be updated on the owning object's status to reflect
// the aggregate state of all resources managed by this component.
//
// Parameters:
//   - conditionType: The condition name (e.g., "WebInterfaceReady").
//
// If the condition type is empty, a validation error is recorded and will be returned by Build().
func (b *Builder) WithConditionType(conditionType ConditionType) *Builder {
	b.conditionTypeSupplied = true

	if conditionType == "" {
		b.buildErrors = append(b.buildErrors, errors.New("condition type cannot be empty"))
	}

	b.component.conditionType = conditionType
	return b
}

// WithResource registers a Kubernetes resource to be managed by this component.
//
// Options configure the resource's lifecycle and its participation in health
// aggregation; see the ResourceOption constructors (ReadOnly, Delete, DeleteWhen,
// GatedBy, Auxiliary, BlockOnAbsence, IgnoreIfAbsent,
// SuppressGraceInconsistencyWarning). With no options the resource is created or
// updated and is required for the component to become Ready.
//
// If a resource with the same Identity() is already registered, or if the
// supplied options fail to resolve (a feature-gate evaluation error or an invalid
// flag combination), a validation error is recorded and returned by Build.
func (b *Builder) WithResource(resource Resource, opts ...ResourceOption) *Builder {
	options, err := resolveResourceOptions(opts)
	if err != nil {
		wrapped := fmt.Errorf("resource %q in component %q: %w", resource.Identity(), b.component.name, err)
		b.resourceErrors[resource.Identity()] = wrapped
		return b
	}

	if _, ok := b.component.resourceLookup[resource.Identity()]; ok {
		b.buildErrors = append(
			b.buildErrors,
			fmt.Errorf(
				"duplicate resource %q in component %q (delete=%t, readOnly=%t, mode=%s)",
				resource.Identity(),
				b.component.name, options.Delete, options.ReadOnly, options.ParticipationMode,
			),
		)
		return b
	}

	// Clear any prior resolution error for this identity: a successful
	// re-registration supersedes the earlier failure.
	delete(b.resourceErrors, resource.Identity())

	b.component.resourceLookup[resource.Identity()] = resource

	if options.Delete {
		b.component.deleteResources = append(b.component.deleteResources, resource)
	} else {
		b.component.reconcileResources = append(b.component.reconcileResources, reconcileEntry{
			Resource: resource,
			Options:  options,
		})
	}

	return b
}

// IncludeWhen registers build()'s resource only when include is true.
//
// When include is false the resource is omitted entirely (not created, read, or
// deleted), build is never called, and the resource takes no part in the
// duplicate-Identity() check. Because build runs only when include is true, it
// may safely dereference the optional inputs that determined include. Contrast
// DeleteWhen, which removes an already-built resource from the cluster.
//
// When include is true the resource is registered exactly as WithResource with
// the same options.
func (b *Builder) IncludeWhen(include bool, build func() Resource, opts ...ResourceOption) *Builder {
	if !include {
		return b
	}
	return b.WithResource(build(), opts...)
}

// WithGracePeriod configures a grace duration for the component's convergence to a Ready state.
//
// When a component is not Ready, it is considered to be in a Progressing state (e.g., Creating,
// Updating, Scaling). The grace period defines how long the component is allowed to remain
// in these Progressing states before it is considered Degraded or Down.
//
// Once the grace period expires:
//   - If the aggregate resource state is Down or Degraded, the component condition
//     transitions to that state.
//   - Resources that implement the Alive interface provide their specific grace status
//     used for this aggregation.
//
// Parameters:
//   - gracePeriod: The duration to allow for convergence. Must be non-negative.
func (b *Builder) WithGracePeriod(gracePeriod time.Duration) *Builder {
	if gracePeriod < 0 {
		b.buildErrors = append(b.buildErrors, fmt.Errorf("grace period must be positive"))
		return b
	}
	b.component.gracePeriod = gracePeriod
	return b
}

// WithFeatureGate sets a feature gate that controls whether the component is active.
//
// When the gate is disabled, the component deletes all of its resources and reports
// a True condition with reason Disabled. When the gate is enabled (or not set),
// the component reconciles normally.
//
// A disabled gate takes precedence over suspension: if the gate is disabled, the
// component is removed regardless of the suspension flag.
//
// A nil gate is ignored. Calling WithFeatureGate more than once records a
// validation error that is returned by Build.
func (b *Builder) WithFeatureGate(gate feature.Gate) *Builder {
	if gate == nil {
		return b
	}

	if b.featureGateSupplied {
		b.buildErrors = append(b.buildErrors, errors.New("feature gate already set; WithFeatureGate can only be called once"))
		return b
	}

	b.featureGateSupplied = true
	b.component.featureGate = gate
	return b
}

// WithPrerequisite registers an initialization barrier for the component.
//
// Prerequisites are evaluated before any resources are reconciled or suspended.
// The barrier remains active while the condition reason is Unknown,
// PrerequisiteNotMet, Disabled, or FeatureGateError. Once the condition
// reason changes to any other value, all prerequisites are permanently
// passed and never re-evaluated.
//
// Multiple prerequisites may be registered; all must be met before the component
// proceeds. They are evaluated in registration order and the first unmet
// prerequisite short-circuits the check.
//
// A nil prerequisite is ignored.
func (b *Builder) WithPrerequisite(prereq Prerequisite) *Builder {
	if prereq == nil {
		return b
	}

	b.component.prerequisites = append(b.component.prerequisites, prereq)
	return b
}

// Suspend allows marking the component as suspended.
//
// By default, the component is not suspended, which equates to passing false to this method.
// When true is passed, all resource within the component that implement the concepts.Suspendable interface are
// suspended.
func (b *Builder) Suspend(suspend bool) *Builder {
	b.component.suspended = suspend

	return b
}
