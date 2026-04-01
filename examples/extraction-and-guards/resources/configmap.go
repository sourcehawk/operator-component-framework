// Package resources provides resource factories for the extraction-and-guards example.
package resources

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseConfigMap returns the desired-state ConfigMap representing database
// connection config.
func BaseConfigMap(owner *app.ExampleApp) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-db-config",
			Namespace: owner.Namespace,
			Labels:    map[string]string{"app": owner.Name},
		},
		Data: map[string]string{
			"db-host": "postgres.default.svc",
			"db-port": "5432",
		},
	}
}

// NewConfigMapResource constructs a ConfigMap for database config. After
// reconciliation, the data extractor captures the db-host value into the
// provided pointer so downstream resources can use it.
func NewConfigMapResource(owner *app.ExampleApp, dbHost *string) (component.Resource, error) {
	builder := configmap.NewBuilder(BaseConfigMap(owner))

	builder.WithDataExtractor(func(cm corev1.ConfigMap) error {
		*dbHost = cm.Data["db-host"]
		fmt.Printf("  Extracted db-host: %q\n", *dbHost)
		return nil
	})

	return builder.Build()
}
