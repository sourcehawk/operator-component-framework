package service

import "github.com/sourcehawk/operator-component-framework/pkg/component/concepts"

// The framework discovers read-only data-extraction support by type-asserting
// the registered Resource to concepts.ObservationRecorder. Because the wrapper
// holds its base resource behind a named (non-embedded) field, the
// RecordObservation method must be forwarded explicitly. This compile-time
// check guards against silent regressions of issue #118.
var _ concepts.ObservationRecorder = (*Resource)(nil)
