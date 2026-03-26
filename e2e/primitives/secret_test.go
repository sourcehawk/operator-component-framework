//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/secret"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseSecret(namespace, name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

var _ = Describe("secret Primitive", Label("secret"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-secret-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a Secret and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				s := newBaseSecret(ns, "app-secret", map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("s3cret"),
				})
				return secret.NewBuilder(s).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the Secret exists with correct data")
			var s corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-secret", Namespace: ns}, &s)).To(Succeed())
			Expect(s.Data).To(HaveKeyWithValue("username", []byte("admin")))
			Expect(s.Data).To(HaveKeyWithValue("password", []byte("s3cret")))

			By("verifying owner reference is set")
			Expect(s.OwnerReferences).NotTo(BeEmpty())
			Expect(s.OwnerReferences[0].Kind).To(Equal("ClusterTestApp"))
			Expect(s.OwnerReferences[0].Name).To(Equal(name))
		})
	})

	Context("Mutations", func() {
		It("should apply data mutations to the Secret", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				s := newBaseSecret(ns, "app-secret-mutated", map[string][]byte{
					"base-key": []byte("base-value"),
				})
				return secret.NewBuilder(s).
					WithMutation(secret.Mutation{
						Name: "add-extra-key",
						Mutate: func(m *secret.Mutator) error {
							m.SetData("injected-key", []byte("injected-value"))
							return nil
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying both base and mutated data are present")
			var s corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-secret-mutated", Namespace: ns}, &s)).To(Succeed())
			Expect(s.Data).To(HaveKeyWithValue("base-key", []byte("base-value")))
			Expect(s.Data).To(HaveKeyWithValue("injected-key", []byte("injected-value")))
		})
	})

	Context("Updates", func() {
		It("should propagate data changes on re-reconciliation", func() {
			var useUpdatedData bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				data := map[string][]byte{"key": []byte("original")}
				if useUpdatedData {
					data = map[string][]byte{"key": []byte("updated"), "new-key": []byte("new-value")}
				}
				s := newBaseSecret(ns, "app-secret-update", data)
				return secret.NewBuilder(s).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial data")
			var s corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-secret-update", Namespace: ns}, &s)).To(Succeed())
			Expect(s.Data).To(HaveKeyWithValue("key", []byte("original")))

			By("switching desired data and triggering reconciliation")
			useUpdatedData = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-data"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated data")
			Eventually(func(g Gomega) map[string][]byte {
				var updated corev1.Secret
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-secret-update", Namespace: ns}, &updated)).To(Succeed())
				return updated.Data
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(And(
				HaveKeyWithValue("key", []byte("updated")),
				HaveKeyWithValue("new-key", []byte("new-value")),
			))
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				s := newBaseSecret(ns, "app-secret-error", map[string][]byte{
					"key": []byte("value"),
				})
				return secret.NewBuilder(s).
					WithMutation(secret.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *secret.Mutator) error {
							return fmt.Errorf("intentional e2e mutation failure")
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Error condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Error"))
		})
	})
})
