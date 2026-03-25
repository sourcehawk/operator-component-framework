package app

import (
	sharedapp "github.com/sourcehawk/operator-component-framework/examples/shared/app"
)

// ExampleApp re-exports the shared CRD type so callers in this package need no import alias.
type ExampleApp = sharedapp.ExampleApp

// ExampleAppSpec re-exports the shared spec type.
type ExampleAppSpec = sharedapp.ExampleAppSpec

// ExampleAppStatus re-exports the shared status type.
type ExampleAppStatus = sharedapp.ExampleAppStatus

// ExampleAppList re-exports the shared list type.
type ExampleAppList = sharedapp.ExampleAppList

// AddToScheme registers the ExampleApp types with the given scheme.
var AddToScheme = sharedapp.AddToScheme
