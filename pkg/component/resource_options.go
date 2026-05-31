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
type ResourceOption func(*resourceConfig)

// resourceConfig is the mutable target that ResourceOptions write into before
// resolution.
type resourceConfig struct {
	readOnly                          bool
	deleteConditions                  []bool
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
	// ReadOnly reports that the resource is read-only.
	ReadOnly bool
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
	// condition or observation is recorded, the data extractor is not invoked, and
	// reconciliation of subsequent resources continues. Last-known state is
	// preserved across an absence. Mutually exclusive with BlockOnAbsence.
	IgnoreIfAbsent bool
}

// ReadOnly marks the resource as read-only: the component fetches its current
// state but never creates or updates it. If the resource is also marked for
// deletion (a disabled GatedBy gate or a true DeleteWhen condition), deletion
// takes precedence and ReadOnly is forced off.
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
// supplied condition is true (or a GatedBy gate is disabled). When a resource is
// deleted, ReadOnly is forced off (deletion takes precedence).
func DeleteWhen(condition bool) ResourceOption {
	return func(c *resourceConfig) { c.deleteConditions = append(c.deleteConditions, condition) }
}

// GatedBy gates the resource's existence on a feature.Gate. When the gate is
// disabled the resource is marked for deletion; when enabled (or nil) the
// resource is managed normally. A gate whose evaluation fails produces a
// resolution error returned by Build.
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
		opt(cfg)
	}
	return cfg.resolve()
}

// resolve validates the configuration, evaluates the feature gate and delete
// conditions, applies deletion precedence, and returns the resolved options.
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

	mode := c.participationMode
	if mode == "" {
		mode = ParticipationModeRequired
	}

	return resourceOptions{
		Delete:                            shouldDelete,
		ReadOnly:                          c.readOnly && !shouldDelete,
		ParticipationMode:                 mode,
		SuppressGraceInconsistencyWarning: c.suppressGraceInconsistencyWarning,
		BlockOnAbsence:                    c.blockOnAbsence,
		IgnoreIfAbsent:                    c.ignoreIfAbsent,
	}, nil
}
