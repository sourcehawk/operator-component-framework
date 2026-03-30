//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/static"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newUnstructuredConfigMap(namespace, name string, data map[string]string) *uns.Unstructured {
	obj := &uns.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
	if data != nil {
		dataMap := make(map[string]interface{}, len(data))
		for k, v := range data {
			dataMap[k] = v
		}
		obj.Object["data"] = dataMap
	}
	return obj
}

var _ = Describe("Unstructured Static Primitive", Label("unstructured-static"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-uns-static-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create an unstructured ConfigMap and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cm := newUnstructuredConfigMap(ns, "uns-static-config", map[string]string{
					"key-a": "value-a",
					"key-b": "value-b",
				})
				return static.NewBuilder(cm).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ConfigMap exists with correct data")
			got := &uns.Unstructured{}
			got.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "uns-static-config", Namespace: ns}, got)).To(Succeed())
			gotData, _, _ := uns.NestedStringMap(got.Object, "data")
			Expect(gotData).To(HaveKeyWithValue("key-a", "value-a"))
			Expect(gotData).To(HaveKeyWithValue("key-b", "value-b"))

			By("verifying owner reference is set")
			refs := got.GetOwnerReferences()
			Expect(refs).NotTo(BeEmpty())
			found := false
			for _, ref := range refs {
				if ref.Kind == "ClusterTestApp" && ref.Name == name {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected owner reference with Kind=ClusterTestApp and Name=%s", name)
		})
	})

	Context("Mutations", func() {
		It("should apply content mutations to the unstructured ConfigMap", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cm := newUnstructuredConfigMap(ns, "uns-static-mutated", map[string]string{
					"base-key": "base-value",
				})
				return static.NewBuilder(cm).
					WithMutation(unstruct.Mutation{
						Name: "add-extra-key",
						Mutate: func(m *unstruct.Mutator) error {
							m.EditContent(func(e *editors.UnstructuredContentEditor) error {
								return e.SetNestedField("injected-value", "data", "injected-key")
							})
							return nil
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying both base and mutated data are present")
			got := &uns.Unstructured{}
			got.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "uns-static-mutated", Namespace: ns}, got)).To(Succeed())
			gotData, _, _ := uns.NestedStringMap(got.Object, "data")
			Expect(gotData).To(HaveKeyWithValue("base-key", "base-value"))
			Expect(gotData).To(HaveKeyWithValue("injected-key", "injected-value"))
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cm := newUnstructuredConfigMap(ns, "uns-static-error", nil)
				return static.NewBuilder(cm).
					WithMutation(unstruct.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *unstruct.Mutator) error {
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

	Context("Guards", func() {
		It("should report Blocked condition when guard blocks", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cm := newUnstructuredConfigMap(ns, "uns-static-guarded", map[string]string{"key": "value"})
				return static.NewBuilder(cm).
					WithGuard(func(_ uns.Unstructured) (concepts.GuardStatusWithReason, error) {
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
