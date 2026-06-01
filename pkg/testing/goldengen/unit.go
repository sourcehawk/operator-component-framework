// Package goldengen is a test-only helper that sweeps a consumer-supplied version
// universe, classifies versions into behaviorally-distinct gating regimes by
// firing-set, generates the minimal goldens covering them, and asserts per-fixture
// mutation gating.
//
// It is opt-in: a consumer that does not import it pays nothing, and the core
// Build/ApplyIntent path never references it.
package goldengen

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/testing/golden"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Unit is a built resource or component the generator can introspect and render.
type Unit interface {
	// RegisteredMutations returns the deduplicated Names of every registered mutation.
	RegisteredMutations() []string
	// FiringSet returns the Names of mutations enabled at the built version.
	FiringSet() ([]string, error)
	// RenderYAML renders the unit's desired state as canonical golden YAML.
	RenderYAML() ([]byte, error)
}

// ResourcePreviewer is a built single-resource primitive: it can be introspected
// and rendered to one client.Object. Every built-in primitive satisfies it.
type ResourcePreviewer interface {
	concepts.MutationInspector
	concepts.Previewable
}

// ComponentPreviewer is a built component: introspectable and rendered to many
// client.Objects. *component.Component satisfies it.
type ComponentPreviewer interface {
	RegisteredMutations() []string
	FiringSet() ([]string, error)
	Preview() ([]client.Object, error)
}

type resourceUnit struct {
	res    ResourcePreviewer
	scheme *runtime.Scheme
}

// Resource adapts a built primitive resource to a Unit, serializing through the
// golden package with the given scheme.
func Resource(res ResourcePreviewer, scheme *runtime.Scheme) Unit {
	return resourceUnit{res: res, scheme: scheme}
}

func (u resourceUnit) RegisteredMutations() []string { return u.res.RegisteredMutations() }
func (u resourceUnit) FiringSet() ([]string, error)  { return u.res.FiringSet() }

func (u resourceUnit) RenderYAML() ([]byte, error) {
	obj, err := u.res.Preview()
	if err != nil {
		return nil, fmt.Errorf("preview resource: %w", err)
	}
	return golden.Serialize(obj, u.scheme)
}

type componentUnit struct {
	comp   ComponentPreviewer
	scheme *runtime.Scheme
}

// Component adapts a built component to a Unit, serializing its managed resources
// into a multi-document YAML stream through the golden package.
func Component(comp ComponentPreviewer, scheme *runtime.Scheme) Unit {
	return componentUnit{comp: comp, scheme: scheme}
}

func (u componentUnit) RegisteredMutations() []string { return u.comp.RegisteredMutations() }
func (u componentUnit) FiringSet() ([]string, error)  { return u.comp.FiringSet() }

func (u componentUnit) RenderYAML() ([]byte, error) {
	objs, err := u.comp.Preview()
	if err != nil {
		return nil, fmt.Errorf("preview component: %w", err)
	}
	return golden.SerializeComponent(objs, u.scheme)
}
