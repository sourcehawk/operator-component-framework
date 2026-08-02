package resources

import (
	"github.com/sourcehawk/operator-component-framework/examples/extraction-and-guards/app"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/secret"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BaseSecret returns the desired-state Secret representing database credentials.
func BaseSecret(owner *app.ExampleApp) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      owner.Name + "-db-credentials",
			Namespace: owner.Namespace,
			Labels:    map[string]string{"app": owner.Name},
		},
		StringData: map[string]string{
			"username": "app-user",
			"password": "changeme",
		},
	}
}

// NewSecretResource constructs a Secret for database credentials. A declared
// data guard blocks it until the db-host cell has been extracted from the
// preceding ConfigMap, and a mutation copies the extracted host into the
// Secret so the credentials and endpoint travel together.
func NewSecretResource(owner *app.ExampleApp, dbHost *concepts.Data[string]) (component.Resource, error) {
	builder := secret.NewBuilder(BaseSecret(owner))

	builder.WithDataGuard(dbHost)
	builder.WithMutation(secret.Mutation{
		Name: "db-host-entry",
		Mutate: func(m *secret.Mutator) error {
			host, err := dbHost.Require()
			if err != nil {
				return err
			}
			m.SetStringData("db-host", host)
			return nil
		},
	})

	return builder.Build()
}
