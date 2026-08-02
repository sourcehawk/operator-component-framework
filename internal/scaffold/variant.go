// Package scaffold renders custom-resource wrapper packages from embedded templates.
package scaffold

// Variant identifies which resource category a generated wrapper belongs to.
type Variant string

// The four resource categories the framework defines.
const (
	// VariantStatic is a configuration object with no runtime health semantics.
	VariantStatic Variant = "static"
	// VariantWorkload is a long-running process with replica-based health.
	VariantWorkload Variant = "workload"
	// VariantTask is a run-to-completion workload.
	VariantTask Variant = "task"
	// VariantIntegration is an external-dependency object such as a Service or Ingress.
	VariantIntegration Variant = "integration"
)

// Variants lists every supported variant in flag-documentation order.
var Variants = []Variant{VariantStatic, VariantWorkload, VariantTask, VariantIntegration}

// convergingStatusMethod is the resource method every status-bearing variant
// forwards its status through. Only the result type differs per variant.
const convergingStatusMethod = "ConvergingStatus"

// VariantSpec describes how a variant wires into pkg/generic. Templates read it
// instead of branching on the variant name.
type VariantSpec struct {
	// GenericBuilder is the pkg/generic builder type, for example "WorkloadBuilder".
	GenericBuilder string
	// GenericConstructor is the pkg/generic builder constructor, for example "NewWorkloadBuilder".
	GenericConstructor string
	// GenericResource is the pkg/generic resource type, for example "WorkloadResource".
	GenericResource string
	// HasStatus reports whether the variant has a required status handler.
	HasStatus bool
	// StatusSetter is the builder method registering the status handler.
	StatusSetter string
	// StatusMethod is the resource method forwarding the status, always
	// convergingStatusMethod.
	StatusMethod string
	// StatusResult is the qualified status result type.
	StatusResult string
	// StatusHandler is the generated default handler's name.
	StatusHandler string
	// StatusConstant is the qualified healthy status constant the default reports.
	StatusConstant string
	// StatusValue is the runtime string value of StatusConstant.
	StatusValue string
	// StatusNoun names the state the handler reports on, used in GoDoc.
	StatusNoun string
	// HasGrace reports whether the variant supports a grace status handler.
	HasGrace bool
	// HasSuspension reports whether the variant supports suspension handlers.
	HasSuspension bool
	// LifecycleInterfaces are the variant-specific bullets of the generated
	// Resource's "It implements the following component interfaces" list, each
	// rendered as "<interface>: <what it is for>." after the component.Resource
	// bullet and before the ones every variant shares.
	LifecycleInterfaces []string
}

// Spec returns the generic-layer wiring for the variant. The zero VariantSpec is
// returned for an unknown variant; Options.Resolve rejects those before rendering.
func (v Variant) Spec() VariantSpec {
	switch v {
	case VariantStatic:
		return VariantSpec{
			GenericBuilder:     "StaticBuilder",
			GenericConstructor: "NewStaticBuilder",
			GenericResource:    "StaticResource",
		}
	case VariantWorkload:
		return VariantSpec{
			GenericBuilder:     "WorkloadBuilder",
			GenericConstructor: "NewWorkloadBuilder",
			GenericResource:    "WorkloadResource",
			HasStatus:          true,
			StatusSetter:       "WithCustomConvergeStatus",
			StatusMethod:       convergingStatusMethod,
			StatusResult:       "concepts.AliveStatusWithReason",
			StatusHandler:      "DefaultConvergingStatusHandler",
			StatusConstant:     "concepts.AliveConvergingStatusHealthy",
			StatusValue:        "Healthy",
			StatusNoun:         "converged",
			HasGrace:           true,
			HasSuspension:      true,
			LifecycleInterfaces: []string{
				"concepts.Alive: for health and readiness tracking.",
				"concepts.Graceful: for health reporting once the grace period expires.",
			},
		}
	case VariantTask:
		return VariantSpec{
			GenericBuilder:     "TaskBuilder",
			GenericConstructor: "NewTaskBuilder",
			GenericResource:    "TaskResource",
			HasStatus:          true,
			StatusSetter:       "WithCustomConvergeStatus",
			StatusMethod:       convergingStatusMethod,
			StatusResult:       "concepts.CompletionStatusWithReason",
			StatusHandler:      "DefaultConvergingStatusHandler",
			StatusConstant:     "concepts.CompletionStatusCompleted",
			StatusValue:        "Completed",
			StatusNoun:         "completed",
			HasSuspension:      true,
			LifecycleInterfaces: []string{
				"concepts.Completable: for run-to-completion tracking.",
			},
		}
	case VariantIntegration:
		return VariantSpec{
			GenericBuilder:     "IntegrationBuilder",
			GenericConstructor: "NewIntegrationBuilder",
			GenericResource:    "IntegrationResource",
			HasStatus:          true,
			StatusSetter:       "WithCustomOperationalStatus",
			StatusMethod:       convergingStatusMethod,
			StatusResult:       "concepts.OperationalStatusWithReason",
			StatusHandler:      "DefaultOperationalStatusHandler",
			StatusConstant:     "concepts.OperationalStatusOperational",
			StatusValue:        "Operational",
			StatusNoun:         "operational",
			HasGrace:           true,
			HasSuspension:      true,
			LifecycleInterfaces: []string{
				"concepts.Operational: for external-dependency readiness tracking.",
				"concepts.Graceful: for health reporting once the grace period expires.",
			},
		}
	default:
		return VariantSpec{}
	}
}
