//go:build e2e

package primitives

import (
	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
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

var _ = Describe("ConfigMap Primitive", func() {
	var (
		ns  string
		key types.NamespacedName
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-cm-")
	})

	AfterEach(func() {
		reconciler.Unregister(key)
	})

	Context("Creation", func() {
		It("should create a ConfigMap and reach Healthy condition", func() {
			name := "cm-create"
			key = types.NamespacedName{Namespace: ns, Name: name}

			reconciler.RegisterResource(key, func(owner *framework.TestApp) (component.Resource, error) {
				cm := newBaseConfigMap(ns, "app-config", map[string]string{
					"setting-a": "value-a",
					"setting-b": "value-b",
				})
				return configmap.NewBuilder(cm).Build()
			})

			framework.NewTestApp(ctx, k8sClient, ns, name)

			Eventually(framework.GetCondition(ctx, k8sClient, key, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ConfigMap exists with correct data")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("setting-a", "value-a"))
			Expect(cm.Data).To(HaveKeyWithValue("setting-b", "value-b"))

			By("verifying owner reference is set")
			Expect(cm.OwnerReferences).NotTo(BeEmpty())
			Expect(cm.OwnerReferences[0].Kind).To(Equal("TestApp"))
			Expect(cm.OwnerReferences[0].Name).To(Equal(name))
		})
	})

	Context("Mutations", func() {
		It("should apply data mutations to the ConfigMap", func() {
			name := "cm-mutate"
			key = types.NamespacedName{Namespace: ns, Name: name}

			reconciler.RegisterResource(key, func(owner *framework.TestApp) (component.Resource, error) {
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

			framework.NewTestApp(ctx, k8sClient, ns, name)

			Eventually(framework.GetCondition(ctx, k8sClient, key, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
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
			name := "cm-update"
			key = types.NamespacedName{Namespace: ns, Name: name}

			var useUpdatedData bool

			reconciler.RegisterResource(key, func(owner *framework.TestApp) (component.Resource, error) {
				data := map[string]string{"key": "original"}
				if useUpdatedData {
					data = map[string]string{"key": "updated", "new-key": "new-value"}
				}
				cm := newBaseConfigMap(ns, "app-config-update", data)
				return configmap.NewBuilder(cm).Build()
			})

			app := framework.NewTestApp(ctx, k8sClient, ns, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetCondition(ctx, k8sClient, key, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial data")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-config-update", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("key", "original"))

			By("switching desired data and triggering reconciliation")
			useUpdatedData = true
			Expect(k8sClient.Get(ctx, key, app)).To(Succeed())
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
})
