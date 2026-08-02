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
	// StatusMethod is the resource method forwarding the status, always "ConvergingStatus".
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
			StatusMethod:       "ConvergingStatus",
			StatusResult:       "concepts.AliveStatusWithReason",
			StatusHandler:      "DefaultConvergingStatusHandler",
			StatusConstant:     "concepts.AliveConvergingStatusHealthy",
			StatusValue:        "Healthy",
			StatusNoun:         "converged",
			HasGrace:           true,
			HasSuspension:      true,
		}
	case VariantTask:
		return VariantSpec{
			GenericBuilder:     "TaskBuilder",
			GenericConstructor: "NewTaskBuilder",
			GenericResource:    "TaskResource",
			HasStatus:          true,
			StatusSetter:       "WithCustomConvergeStatus",
			StatusMethod:       "ConvergingStatus",
			StatusResult:       "concepts.CompletionStatusWithReason",
			StatusHandler:      "DefaultConvergingStatusHandler",
			StatusConstant:     "concepts.CompletionStatusCompleted",
			StatusValue:        "Completed",
			StatusNoun:         "completed",
			HasSuspension:      true,
		}
	case VariantIntegration:
		return VariantSpec{
			GenericBuilder:     "IntegrationBuilder",
			GenericConstructor: "NewIntegrationBuilder",
			GenericResource:    "IntegrationResource",
			HasStatus:          true,
			StatusSetter:       "WithCustomOperationalStatus",
			StatusMethod:       "ConvergingStatus",
			StatusResult:       "concepts.OperationalStatusWithReason",
			StatusHandler:      "DefaultOperationalStatusHandler",
			StatusConstant:     "concepts.OperationalStatusOperational",
			StatusValue:        "Operational",
			StatusNoun:         "operational",
			HasGrace:           true,
			HasSuspension:      true,
		}
	default:
		return VariantSpec{}
	}
}
