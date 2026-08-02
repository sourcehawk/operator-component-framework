package component

import (
	"errors"

	"github.com/sourcehawk/operator-component-framework/pkg/feature"
)

// ResourceOption configures how a single resource is managed within a component.
//
// Options are supplied to Builder.WithResource and Builder.IncludeWhen and are
// resolved at registration time. Any resolution error (a failed feature-gate
// evaluation or an invalid combination of flags) is recorded on the component
// builder and returned by Build.
//
// Options operate on the lifecycle axis: they describe what to do with a
// resource that is present in the reconcile set. Whether a resource is present
// at all is the presence axis, controlled by WithResource (always present) and
// IncludeWhen (present only when a condition holds).
//
// A nil ResourceOption is ignored, so a conditionally-assigned option
// (var opt ResourceOption; ...) may be passed without a nil check.
type ResourceOption func(*resourceConfig)

// resourceConfig is the mutable target that ResourceOption functions write into
// before resolution.
type resourceConfig struct {
	readOnly                          bool
	unowned                           bool
	deleteConditions                  []bool
	orphanConditions                  []bool
	gate                              feature.Gate
	participationMode                 ParticipationMode
	blockOnAbsence                    bool
	ignoreIfAbsent                    bool
	suppressGraceInconsistencyWarning bool
}

// resourceOptions is the resolved configuration for a single managed resource,
// produced from a resourceConfig by resolve and stored on reconcileEntry. It
// controls the resource's lifecycle (creation, deletion, or read-only) and its
// participation in the component's health aggregation.
type resourceOptions struct {
	// Delete reports that the resource is marked for deletion during reconciliation.
	Delete bool
	// Orphan reports that the resource is released during reconciliation: the
	// component removes its controller owner reference and stops managing it,
	// leaving the object in the cluster.
	Orphan bool
	// ReadOnly reports that the resource is read-only.
	ReadOnly bool
	// Unowned reports that the component must not set a controller owner reference
	// on this resource. The resource is still created and updated normally, but
	// Kubernetes will not garbage-collect it when the owner CR is deleted. Use this
	// for resources that must outlive the owner, such as backup records.
	Unowned bool
	// ParticipationMode describes how the resource participates in the component
	// health aggregation. Defaults to ParticipationModeRequired.
	ParticipationMode ParticipationMode
	// SuppressGraceInconsistencyWarning suppresses the warning log emitted when the
	// resource's grace status handler returns Healthy while its convergence handler
	// returns non-healthy.
	SuppressGraceInconsistencyWarning bool
	// BlockOnAbsence applies to read-only resources. When true, a NotFound response
	// is treated as a guard-blocked condition rather than an error. Mutually
	// exclusive with IgnoreIfAbsent.
	BlockOnAbsence bool
	// IgnoreIfAbsent applies to read-only resources. When true, a NotFound response
	// when reading the resource is silently ignored: the entry is skipped, no
	// condition or observation is recorded, no declared data extraction is run, and
	// reconciliation of subsequent resources continues. Last-known state is
	// preserved across an absence. Mutually exclusive with BlockOnAbsence.
	IgnoreIfAbsent bool
}

// ReadOnly marks the resource as read-only: the component fetches its current
// state but never creates, updates, or deletes it. A read-only resource is not
// owned by the component, so it is mutually exclusive with every deletion
// trigger: combining ReadOnly with Delete, DeleteWhen, or GatedBy is a
// configuration error returned by Build. To conditionally include a read-only
// resource, use IncludeWhen, which omits the resource without ever deleting it.
func ReadOnly() ResourceOption {
	return func(c *resourceConfig) { c.readOnly = true }
}

// Delete marks the resource for unconditional removal from the cluster. It is
// shorthand for DeleteWhen(true).
func Delete() ResourceOption {
	return DeleteWhen(true)
}

// DeleteWhen marks the resource for removal from the cluster when condition is
// true. Calls are additive with OR semantics: the resource is deleted if any
// supplied condition is true (or a GatedBy gate is disabled). Mutually exclusive
// with ReadOnly, since a read-only resource is never deleted; combining them is
// a configuration error returned by Build.
func DeleteWhen(condition bool) ResourceOption {
	return func(c *resourceConfig) { c.deleteConditions = append(c.deleteConditions, condition) }
}

// OrphanWhen releases the resource from the component when condition is true: the
// component stops managing it and removes the controller owner reference it set,
// leaving the object in the cluster rather than deleting it. Use it to migrate a
// resource to a new owner without deleting it.
//
// Calls are additive with OR semantics: the resource is orphaned if any supplied
// condition is true. OrphanWhen is mutually exclusive with Delete, DeleteWhen,
// GatedBy, and ReadOnly; combining any of them is a configuration error returned
// by Build.
func OrphanWhen(condition bool) ResourceOption {
	return func(c *resourceConfig) { c.orphanConditions = append(c.orphanConditions, condition) }
}

// GatedBy conditionally renders an owned resource based on a feature.Gate: it is
// the option to reach for when a resource the component owns should exist for
// some feature states and be removed for others. When the gate is disabled the
// resource is marked for deletion; when enabled it is managed normally. A nil
// gate is a no-op (treated as always enabled). A gate whose evaluation fails
// produces a resolution error returned by Build.
//
// Because a disabled gate deletes the resource, GatedBy is mutually exclusive
// with ReadOnly; combining them is a configuration error returned by Build. For
// an optional resource that must never be deleted (a read-only reference owned
// by someone else), use IncludeWhen instead, which omits rather than deletes.
func GatedBy(gate feature.Gate) ResourceOption {
	return func(c *resourceConfig) { c.gate = gate }
}

// Auxiliary sets the resource's participation mode to Auxiliary, excluding its
// health from the component's aggregation.
func Auxiliary() ResourceOption {
	return func(c *resourceConfig) { c.participationMode = ParticipationModeAuxiliary }
}

// BlockOnAbsence opts a read-only resource into guard-blocked semantics when the
// cluster reports NotFound: a blocked status ("waiting for <resource>") is
// recorded and the remaining resources are short-circuited, instead of erroring
// back through controller-runtime's exponential backoff. Requires ReadOnly and
// is mutually exclusive with IgnoreIfAbsent; Build returns an error otherwise.
func BlockOnAbsence() ResourceOption {
	return func(c *resourceConfig) { c.blockOnAbsence = true }
}

// IgnoreIfAbsent opts a read-only resource into optional semantics: a NotFound
// from the cluster is silently ignored and reconciliation continues. Requires
// ReadOnly and is mutually exclusive with BlockOnAbsence; Build returns an error
// otherwise.
func IgnoreIfAbsent() ResourceOption {
	return func(c *resourceConfig) { c.ignoreIfAbsent = true }
}

// Unowned marks the resource as unowned: the component creates and updates it
// normally, but does not set a controller owner reference. Without an owner
// reference, Kubernetes will not garbage-collect the resource when the owner CR
// is deleted. Use this for resources that must outlive the owner, such as backup
// records.
func Unowned() ResourceOption {
	return func(c *resourceConfig) { c.unowned = true }
}

// SuppressGraceInconsistencyWarning suppresses the warning log emitted when the
// resource's grace handler returns Healthy while its convergence handler returns
// non-healthy. Use this when the inconsistency is intentional.
func SuppressGraceInconsistencyWarning() ResourceOption {
	return func(c *resourceConfig) { c.suppressGraceInconsistencyWarning = true }
}

// resolveResourceOptions applies opts to a fresh config and resolves it into the
// final resourceOptions, returning any validation or feature-evaluation error.
func resolveResourceOptions(opts []ResourceOption) (resourceOptions, error) {
	cfg := &resourceConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(cfg)
	}
	return cfg.resolve()
}

// resolve validates the configuration, evaluates the feature gate and delete
// conditions, and returns the resolved options.
func (c *resourceConfig) resolve() (resourceOptions, error) {
	if c.blockOnAbsence && c.ignoreIfAbsent {
		return resourceOptions{}, errors.New(
			"resource options BlockOnAbsence and IgnoreIfAbsent are mutually exclusive",
		)
	}
	if c.blockOnAbsence && !c.readOnly {
		return resourceOptions{}, errors.New("resource option BlockOnAbsence requires ReadOnly")
	}
	if c.ignoreIfAbsent && !c.readOnly {
		return resourceOptions{}, errors.New("resource option IgnoreIfAbsent requires ReadOnly")
	}

	// A read-only resource is not owned by the component, so it must never be
	// deleted. Combining ReadOnly with any deletion trigger is a configuration
	// error; use IncludeWhen to conditionally include a read-only resource.
	if c.readOnly && len(c.deleteConditions) > 0 {
		return resourceOptions{}, errors.New(
			"resource option ReadOnly is mutually exclusive with Delete and DeleteWhen; " +
				"use IncludeWhen to conditionally include a read-only resource",
		)
	}
	if c.readOnly && c.gate != nil {
		return resourceOptions{}, errors.New(
			"resource option ReadOnly is mutually exclusive with GatedBy; " +
				"use IncludeWhen to conditionally include a read-only resource",
		)
	}

	if len(c.orphanConditions) > 0 {
		if len(c.deleteConditions) > 0 {
			return resourceOptions{}, errors.New("resource option OrphanWhen is mutually exclusive with Delete and DeleteWhen")
		}
		if c.gate != nil {
			return resourceOptions{}, errors.New("resource option OrphanWhen is mutually exclusive with GatedBy")
		}
		if c.readOnly {
			return resourceOptions{}, errors.New("resource option OrphanWhen is mutually exclusive with ReadOnly")
		}
	}

	shouldDelete := false
	if c.gate != nil {
		enabled, err := c.gate.Enabled()
		if err != nil {
			return resourceOptions{}, err
		}
		if !enabled {
			shouldDelete = true
		}
	}
	if !shouldDelete {
		for _, cond := range c.deleteConditions {
			if cond {
				shouldDelete = true
				break
			}
		}
	}

	shouldOrphan := false
	for _, cond := range c.orphanConditions {
		if cond {
			shouldOrphan = true
			break
		}
	}

	mode := c.participationMode
	if mode == "" {
		mode = ParticipationModeRequired
	}

	// ReadOnly and shouldDelete cannot both be set: the validation above rejects
	// any read-only resource that carries a deletion trigger.
	return resourceOptions{
		Delete:                            shouldDelete,
		Orphan:                            shouldOrphan,
		ReadOnly:                          c.readOnly,
		Unowned:                           c.unowned,
		ParticipationMode:                 mode,
		SuppressGraceInconsistencyWarning: c.suppressGraceInconsistencyWarning,
		BlockOnAbsence:                    c.blockOnAbsence,
		IgnoreIfAbsent:                    c.ignoreIfAbsent,
	}, nil
}
