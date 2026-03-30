//go:build e2e

package primitives

import (
	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseConfigMap(namespace, name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

var _ = Describe("ConfigMap Primitive", Label("configmap"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-cm-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a ConfigMap and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cm := newBaseConfigMap(ns, "app-config", map[string]string{
					"setting-a": "value-a",
					"setting-b": "value-b",
				})
				return configmap.NewBuilder(cm).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ConfigMap exists with correct data")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("setting-a", "value-a"))
			Expect(cm.Data).To(HaveKeyWithValue("setting-b", "value-b"))

			By("verifying owner reference is set")
			expectOwnerReference(cm.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply data mutations to the ConfigMap", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cm := newBaseConfigMap(ns, "app-config-mutated", map[string]string{
					"base-key": "base-value",
				})
				return configmap.NewBuilder(cm).
					WithMutation(configmap.Mutation{
						Name: "add-extra-key",
						Mutate: func(m *configmap.Mutator) error {
							m.SetEntry("injected-key", "injected-value")
							return nil
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying both base and mutated data are present")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config-mutated", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("base-key", "base-value"))
			Expect(cm.Data).To(HaveKeyWithValue("injected-key", "injected-value"))
		})
	})

	Context("Updates", func() {
		It("should propagate data changes on re-reconciliation", func() {
			var useUpdatedData bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				data := map[string]string{"key": "original"}
				if useUpdatedData {
					data = map[string]string{"key": "updated", "new-key": "new-value"}
				}
				cm := newBaseConfigMap(ns, "app-config-update", data)
				return configmap.NewBuilder(cm).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial data")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config-update", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("key", "original"))

			By("switching desired data and triggering reconciliation")
			useUpdatedData = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-data"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated data")
			Eventually(func(g Gomega) map[string]string {
				var updated corev1.ConfigMap
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config-update", Namespace: ns}, &updated)).To(Succeed())
				return updated.Data
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(And(
				HaveKeyWithValue("key", "updated"),
				HaveKeyWithValue("new-key", "new-value"),
			))
		})
	})

	Context("Guards", func() {
		It("should report Blocked condition when guard blocks", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cm := newBaseConfigMap(ns, "guarded-cm", map[string]string{"key": "value"})
				return configmap.NewBuilder(cm).
					WithGuard(func(_ corev1.ConfigMap) (concepts.GuardStatusWithReason, error) {
						return concepts.GuardStatusWithReason{
							Status: concepts.GuardStatusBlocked,
							Reason: "guard test",
						}, nil
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Blocked"))
		})
	})
})
