//go:build e2e

package primitives

import (
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/integration"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func unstructuredAlwaysOperational(_ concepts.ConvergingOperation, _ *uns.Unstructured) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusOperational,
		Reason: "e2e: always operational",
	}, nil
}

func unstructuredAlwaysPendingOp(_ concepts.ConvergingOperation, _ *uns.Unstructured) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusPending,
		Reason: "e2e: always pending",
	}, nil
}

func unstructuredIntegrationGraceDegraded(_ *uns.Unstructured) (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDegraded,
		Reason: "e2e: degraded for test",
	}, nil
}

var _ = Describe("Unstructured Integration Primitive", Label("unstructured-integration"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-unstruct-int-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create an unstructured ConfigMap and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newUnstructuredConfigMap(ns, "int-create", map[string]string{
					"key-a": "val-a",
					"key-b": "val-b",
				})
				return integration.NewBuilder(obj).
					WithCustomOperationalStatus(unstructuredAlwaysOperational).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ConfigMap exists with correct data")
			var obj uns.Unstructured
			obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "int-create", Namespace: ns}, &obj)).To(Succeed())

			data, found, err := uns.NestedStringMap(obj.Object, "data")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(data).To(HaveKeyWithValue("key-a", "val-a"))
			Expect(data).To(HaveKeyWithValue("key-b", "val-b"))

			By("verifying owner reference is set")
			expectOwnerReference(metav1.ObjectMeta{
				OwnerReferences: obj.GetOwnerReferences(),
			}, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply content edit mutations to the unstructured ConfigMap", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newUnstructuredConfigMap(ns, "int-mutated", map[string]string{
					"base-key": "base-value",
				})
				return integration.NewBuilder(obj).
					WithCustomOperationalStatus(unstructuredAlwaysOperational).
					WithMutation(unstruct.Mutation{
						Name: "add-content",
						Mutate: func(m *unstruct.Mutator) error {
							m.EditContent(func(e *editors.UnstructuredContentEditor) error {
								return e.SetNestedString("injected-value", "data", "injected-key")
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
			var obj uns.Unstructured
			obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "int-mutated", Namespace: ns}, &obj)).To(Succeed())

			data, found, err := uns.NestedStringMap(obj.Object, "data")
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			Expect(data).To(HaveKeyWithValue("base-key", "base-value"))
			Expect(data).To(HaveKeyWithValue("injected-key", "injected-value"))
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded after grace period expires for a non-converging resource", func() {
			gracePeriod := 5 * time.Second

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				obj := newUnstructuredConfigMap(ns, "int-grace", map[string]string{
					"status": "pending",
				})

				res, err := integration.NewBuilder(obj).
					WithCustomOperationalStatus(unstructuredAlwaysPendingOp).
					WithCustomGraceStatus(unstructuredIntegrationGraceDegraded).
					Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("e2e-grace").
					WithConditionType("E2EReady").
					WithResource(res).
					WithGracePeriod(gracePeriod).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for the initial condition to be set")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				ShouldNot(BeNil())

			By("waiting for grace period to expire")
			time.Sleep(gracePeriod + 2*time.Second)

			By("triggering re-reconciliation after grace period")
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				if a.Annotations == nil {
					a.Annotations = map[string]string{}
				}
				a.Annotations["e2e.ocf.io/trigger"] = "grace-check"
			})

			By("waiting for Degraded condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Degraded"))
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newUnstructuredConfigMap(ns, "int-error", map[string]string{
					"key": "value",
				})
				return integration.NewBuilder(obj).
					WithCustomOperationalStatus(unstructuredAlwaysOperational).
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
				obj := newUnstructuredConfigMap(ns, "int-guarded", map[string]string{"key": "value"})
				return integration.NewBuilder(obj).
					WithCustomOperationalStatus(unstructuredAlwaysOperational).
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
