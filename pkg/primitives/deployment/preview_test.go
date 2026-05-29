package deployment

import (
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
)

// Compile-time assertion that the deployment Resource satisfies Previewable.
var _ concepts.Previewable = (*Resource)(nil)
