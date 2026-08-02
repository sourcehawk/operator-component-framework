package scaffold

import (
	"fmt"
	"go/token"
	"regexp"
	"strings"
)

var (
	apiVersionPattern   = regexp.MustCompile(`^v[0-9]+((alpha|beta)[0-9]+)?$`)
	apiGroupPattern     = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	exportedNamePattern = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*$`)
	packageNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	identifierPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	nonAlphanumeric     = regexp.MustCompile(`[^a-z0-9]`)
)

// Options are the raw flag values of "ocf scaffold wrapper" before validation
// and defaulting.
type Options struct {
	// Type is the wrapped Go type as "<import-path>.<TypeName>".
	Type string
	// Variant is the resource category name.
	Variant string
	// Group is the API group. An empty Group is valid for core types, so
	// GroupSet records whether the flag was provided at all.
	Group string
	// GroupSet reports whether --group was provided.
	GroupSet bool
	// Version is the API version. Derived from Type when empty.
	Version string
	// Kind is the kind used in the identity string. Defaults to the type name.
	Kind string
	// Alias is the import alias for the wrapped type's package. Derived when empty.
	Alias string
	// Package is the generated Go package name. Defaults to the lowercased kind.
	Package string
	// ClusterScoped marks the wrapped kind as cluster-scoped.
	ClusterScoped bool
}

// Resolve validates the options and derives every unset value, returning the
// data the templates render from.
func (o Options) Resolve() (TemplateData, error) {
	importPath, typeName, err := splitType(o.Type)
	if err != nil {
		return TemplateData{}, err
	}

	variant, err := parseVariant(o.Variant)
	if err != nil {
		return TemplateData{}, err
	}

	if !o.GroupSet {
		return TemplateData{}, fmt.Errorf(`--group is required (pass --group "" for core API group types)`)
	}
	if o.Group != "" && !apiGroupPattern.MatchString(o.Group) {
		return TemplateData{}, fmt.Errorf(
			`--group %q is not a valid API group (a DNS subdomain, or "" for core API group types)`, o.Group,
		)
	}

	version := o.Version
	if version == "" {
		lastSegment := lastPathSegment(importPath)
		if !apiVersionPattern.MatchString(lastSegment) {
			return TemplateData{}, fmt.Errorf(
				"--version is required: the last segment %q of the import path is not an API version",
				lastSegment,
			)
		}
		version = lastSegment
	} else if !apiVersionPattern.MatchString(version) {
		return TemplateData{}, fmt.Errorf("--version %q is not a valid API version", version)
	}

	kind := o.Kind
	if kind == "" {
		kind = typeName
	}
	if !exportedNamePattern.MatchString(kind) {
		return TemplateData{}, fmt.Errorf("--kind %q must be an exported Go identifier", kind)
	}

	alias := o.Alias
	if alias == "" {
		alias = deriveAlias(importPath)
		if alias == "" {
			return TemplateData{}, fmt.Errorf(
				"--alias is required: an import alias cannot be derived from import path %q", importPath,
			)
		}
	}
	if !identifierPattern.MatchString(alias) || token.Lookup(alias).IsKeyword() {
		return TemplateData{}, fmt.Errorf("--alias %q is not a valid Go identifier", alias)
	}

	pkg := o.Package
	if pkg == "" {
		pkg = strings.ToLower(kind)
	}
	if !packageNamePattern.MatchString(pkg) {
		return TemplateData{}, fmt.Errorf("--package %q is not a valid Go package name", pkg)
	}
	if token.Lookup(pkg).IsKeyword() {
		return TemplateData{}, fmt.Errorf("--package %q is a Go keyword", pkg)
	}

	return TemplateData{
		Package:       pkg,
		ImportPath:    importPath,
		ImportAlias:   alias,
		TypeName:      typeName,
		Group:         o.Group,
		Version:       version,
		Kind:          kind,
		ClusterScoped: o.ClusterScoped,
		Variant:       variant,
	}, nil
}

// splitType splits "<import-path>.<TypeName>" on the last dot in its final
// path segment. Dots earlier in the import path, such as the domain in
// "k8s.io/api/apps/v1.Deployment", are not treated as the separator.
func splitType(value string) (importPath, typeName string, err error) {
	if value == "" {
		return "", "", fmt.Errorf("--type is required")
	}

	slashIdx := strings.LastIndex(value, "/")
	idx := strings.LastIndex(value, ".")
	if idx < 0 || idx < slashIdx {
		return "", "", fmt.Errorf("--type must be <import-path>.<TypeName>, got %q", value)
	}

	importPath, typeName = value[:idx], value[idx+1:]
	if importPath == "" {
		return "", "", fmt.Errorf("--type is missing an import path, got %q", value)
	}
	if typeName == "" {
		return "", "", fmt.Errorf("--type must be <import-path>.<TypeName>, got %q", value)
	}
	if !exportedNamePattern.MatchString(typeName) {
		return "", "", fmt.Errorf("--type type name %q must be an exported Go identifier", typeName)
	}

	return importPath, typeName, nil
}

// parseVariant maps the flag value to a Variant.
func parseVariant(value string) (Variant, error) {
	if value == "" {
		return "", fmt.Errorf("--variant is required")
	}

	for _, variant := range Variants {
		if Variant(value) == variant {
			return variant, nil
		}
	}

	names := make([]string, 0, len(Variants))
	for _, variant := range Variants {
		names = append(names, string(variant))
	}

	return "", fmt.Errorf("--variant must be one of %s; got %q", strings.Join(names, ", "), value)
}

// lastPathSegment returns the final slash-separated segment of an import path.
func lastPathSegment(importPath string) string {
	if idx := strings.LastIndex(importPath, "/"); idx >= 0 {
		return importPath[idx+1:]
	}
	return importPath
}

// deriveAlias builds an import alias following the Kubernetes ecosystem
// convention (corev1, appsv1, certmanagerv1): the sanitized second-to-last path
// segment concatenated with a version-like last segment. When the last segment
// is not an API version, the sanitized last segment is used alone. It returns an
// empty string when no valid identifier can be derived.
func deriveAlias(importPath string) string {
	segments := strings.Split(importPath, "/")
	last := sanitizeSegment(segments[len(segments)-1])

	if !apiVersionPattern.MatchString(segments[len(segments)-1]) {
		return validAliasOrEmpty(last)
	}

	if len(segments) < 2 {
		return validAliasOrEmpty(last)
	}

	return validAliasOrEmpty(sanitizeSegment(segments[len(segments)-2]) + last)
}

// sanitizeSegment lowercases a path segment and strips every character that
// cannot appear in a Go identifier.
func sanitizeSegment(segment string) string {
	return nonAlphanumeric.ReplaceAllString(strings.ToLower(segment), "")
}

// validAliasOrEmpty returns alias when it is a usable Go identifier.
func validAliasOrEmpty(alias string) string {
	if alias == "" || !identifierPattern.MatchString(alias) || token.Lookup(alias).IsKeyword() {
		return ""
	}
	return alias
}
