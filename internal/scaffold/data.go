package scaffold

import (
	"fmt"
	"strings"
)

// TemplateData is the fully resolved input to the wrapper templates. Every field
// is validated or derived by Options.Resolve.
type TemplateData struct {
	// Package is the Go package name of the generated package.
	Package string
	// ImportPath is the wrapped type's import path.
	ImportPath string
	// ImportAlias is the alias the generated files import ImportPath under.
	ImportAlias string
	// TypeName is the wrapped Go type's name.
	TypeName string
	// Group is the API group, empty for core types.
	Group string
	// Version is the API version.
	Version string
	// Kind is the kind used in identities and documentation.
	Kind string
	// ClusterScoped reports whether the wrapped kind is cluster-scoped.
	ClusterScoped bool
	// Variant is the resource category the wrapper belongs to.
	Variant Variant
}

// Spec returns the generic-layer wiring for the data's variant.
func (d TemplateData) Spec() VariantSpec {
	return d.Variant.Spec()
}

// QualifiedType returns the wrapped type qualified by its import alias.
func (d TemplateData) QualifiedType() string {
	return fmt.Sprintf("%s.%s", d.ImportAlias, d.TypeName)
}

// PointerType returns a pointer to the wrapped type qualified by its import alias.
func (d TemplateData) PointerType() string {
	return "*" + d.QualifiedType()
}

// LowercaseKind returns the kind in lower case, which is the value the
// framework uses for a resource's `resource` metric label when the wrapper's
// builder is not given an explicit metrics identifier.
func (d TemplateData) LowercaseKind() string {
	return strings.ToLower(d.Kind)
}

// APIVersion returns "<group>/<version>", or bare "<version>" for core types.
func (d TemplateData) APIVersion() string {
	if d.Group == "" {
		return d.Version
	}
	return d.Group + "/" + d.Version
}

// IdentityFormat returns the fmt format string for the resource identity,
// following the framework convention "<apiVersion>/<Kind>/<namespace>/<name>"
// and omitting the namespace segment for cluster-scoped kinds.
func (d TemplateData) IdentityFormat() string {
	if d.ClusterScoped {
		return fmt.Sprintf("%s/%s/%%s", d.APIVersion(), d.Kind)
	}
	return fmt.Sprintf("%s/%s/%%s/%%s", d.APIVersion(), d.Kind)
}

// IdentityArgs returns the fmt arguments matching IdentityFormat, expressed
// against the identity function's parameter named o.
func (d TemplateData) IdentityArgs() string {
	if d.ClusterScoped {
		return "o.Name"
	}
	return "o.Namespace, o.Name"
}
