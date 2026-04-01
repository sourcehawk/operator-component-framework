package resources

import (
	"fmt"

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

// NewSecretResource constructs a Secret for database credentials. A guard
// blocks this resource until the db-host value has been extracted from the
// preceding ConfigMap.
func NewSecretResource(owner *app.ExampleApp, dbHost *string) (component.Resource, error) {
	builder := secret.NewBuilder(BaseSecret(owner))

	builder.WithGuard(func(_ corev1.Secret) (concepts.GuardStatusWithReason, error) {
		if *dbHost == "" {
			fmt.Println("  Guard: blocked, waiting for db-host from ConfigMap")
			return concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusBlocked,
				Reason: "waiting for db-host to be extracted from ConfigMap",
			}, nil
		}

		fmt.Printf("  Guard: unblocked, db-host is %q\n", *dbHost)
		return concepts.GuardStatusWithReason{
			Status: concepts.GuardStatusUnblocked,
		}, nil
	})

	return builder.Build()
}
