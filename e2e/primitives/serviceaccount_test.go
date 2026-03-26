//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/serviceaccount"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseServiceAccount(namespace, name string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

var _ = Describe("ServiceAccount Primitive", Label("serviceaccount"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-serviceaccount-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a ServiceAccount and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				sa := newBaseServiceAccount(ns, "app-sa")
				return serviceaccount.NewBuilder(sa).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ServiceAccount exists")
			var sa corev1.ServiceAccount
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-sa", Namespace: ns}, &sa)).To(Succeed())

			By("verifying owner reference is set")
			expectOwnerReference(sa.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply mutations to the ServiceAccount", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				sa := newBaseServiceAccount(ns, "app-sa-mutated")
				return serviceaccount.NewBuilder(sa).
					WithMutation(serviceaccount.Mutation{
						Name: "add-image-pull-secret",
						Mutate: func(m *serviceaccount.Mutator) error {
							m.EnsureImagePullSecret("my-registry-secret")
							return nil
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the image pull secret mutation was applied")
			var sa corev1.ServiceAccount
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-sa-mutated", Namespace: ns}, &sa)).To(Succeed())
			Expect(sa.ImagePullSecrets).To(ContainElement(corev1.LocalObjectReference{Name: "my-registry-secret"}))
		})
	})

	Context("Updates", func() {
		It("should propagate changes on re-reconciliation", func() {
			var useUpdatedSpec bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				sa := newBaseServiceAccount(ns, "app-sa-update")
				if useUpdatedSpec {
					automount := false
					sa.AutomountServiceAccountToken = &automount
				}
				return serviceaccount.NewBuilder(sa).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial state — automountServiceAccountToken not set")
			var sa corev1.ServiceAccount
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-sa-update", Namespace: ns}, &sa)).To(Succeed())
			Expect(sa.AutomountServiceAccountToken).To(BeNil())

			By("switching desired spec and triggering reconciliation")
			useUpdatedSpec = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-sa"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated state — automountServiceAccountToken is false")
			Eventually(func(g Gomega) *bool {
				var updated corev1.ServiceAccount
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-sa-update", Namespace: ns}, &updated)).To(Succeed())
				return updated.AutomountServiceAccountToken
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(HaveValue(BeFalse()))
		})
	})

	Context("Error", func() {
		It("should report Error condition when mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				sa := newBaseServiceAccount(ns, "app-sa-error")
				return serviceaccount.NewBuilder(sa).
					WithMutation(serviceaccount.Mutation{
						Name: "intentional-failure",
						Mutate: func(m *serviceaccount.Mutator) error {
							return fmt.Errorf("intentional e2e mutation failure")
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Error"))
		})
	})
})
