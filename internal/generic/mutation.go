package generic

import "github.com/sourcehawk/operator-component-framework/pkg/feature"

// MutationFeature is an alias for [feature.MutationFeature].
type MutationFeature = feature.MutationFeature

// Mutation is an alias for [feature.Mutation].
//
// Using the alias throughout internal/generic keeps the internal implementation
// consistent without redefining types that belong to the public API surface.
type Mutation[T any] = feature.Mutation[T]
