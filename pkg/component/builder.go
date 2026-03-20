// Package component provides the core framework for managing Kubernetes resources as logical components.
package component

import (
	"errors"
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// ResourceOptions defines configuration for how a Kubernetes resource is managed
// within a component. It controls the resource's lifecycle (creation, deletion, or read-only)
// and its participation in the component's health aggregation.
type ResourceOptions struct {
	// Delete If true, the resource is marked for deletion during reconciliation.
	Delete bool
	// ReadOnly If true, the resource is read-only
	ReadOnly bool
	// ParticipationMode describes in what way the resource participates in the component health aggregation.
	// By default, Alive and Operational resources use ParticipationModeRequired, while Completable resources
	// are ParticipationModeAuxiliary when not otherwise specified.
	//
	// If the resource is static, e.g. not implementing any of the interfaces mentioned, the mode has no effect,
	// since the resource's status is determined by whether it can be applied or not.
	ParticipationMode ParticipationMode
}

// Builder implements the fluent API for constructing and validating a Component.
// It ensures that a component is configured with consistent rules before it is
// used in a reconciliation loop.
type Builder struct {
	component   *Component
	buildErrors []error

	nameSupplied          bool
	conditionTypeSupplied bool
}

// NewComponentBuilder initializes a new Builder for creating a Component.
//
// A Component manages a single condition on an owning object (the OperatorCRD) and aggregates
// the lifecycle, readiness, and suspension state of all registered resources.
func NewComponentBuilder() *Builder {
	return &Builder{
		component: &Component{
			name:                "",
			suspended:           false,
			conditionType:       "",
			gracePeriod:         time.Duration(0),
			resourceLookup:      make(map[string]Resource),
			participationLookup: make(map[string]ParticipationMode),
		},
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
// If a resource with the same Identity() is already registered, a validation error
// is recorded and will be returned by Build().
func (b *Builder) WithResource(resource Resource, options ResourceOptions) *Builder {
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

	b.component.resourceLookup[resource.Identity()] = resource

	if options.ParticipationMode == "" {
		if _, ok := resource.(concepts.Completable); ok {
			options.ParticipationMode = ParticipationModeAuxiliary
		} else {
			options.ParticipationMode = ParticipationModeRequired
		}
	}

	b.component.participationLookup[resource.Identity()] = options.ParticipationMode

	switch {
	case options.Delete:
		b.component.deleteResources = append(b.component.deleteResources, resource)
	case options.ReadOnly:
		b.component.readResources = append(b.component.readResources, resource)
	default:
		b.component.createResources = append(b.component.createResources, resource)
	}

	return b
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

// Suspend allows marking the component as suspended.
//
// By default, the component is not suspended, which equates to passing false to this method.
// When true is passed, all resource within the component that implement the concepts.Suspendable interface are
// suspended.
func (b *Builder) Suspend(suspend bool) *Builder {
	b.component.suspended = suspend

	return b
}
