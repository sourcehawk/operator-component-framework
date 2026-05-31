//go:build e2e

package primitives

import (
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	unstruct "github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/unstructured/workload"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func unstructuredAlwaysHealthy(_ concepts.ConvergingOperation, _ *uns.Unstructured) (concepts.AliveStatusWithReason, error) {
	return concepts.AliveStatusWithReason{
		Status: concepts.AliveConvergingStatusHealthy,
		Reason: "e2e: always healthy",
	}, nil
}

func unstructuredAlwaysCreating(_ concepts.ConvergingOperation, _ *uns.Unstructured) (concepts.AliveStatusWithReason, error) {
	return concepts.AliveStatusWithReason{
		Status: concepts.AliveConvergingStatusCreating,
		Reason: "e2e: never converges",
	}, nil
}

func unstructuredGraceDegraded(_ *uns.Unstructured) (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDegraded,
		Reason: "e2e: degraded for test",
	}, nil
}

var _ = Describe("Unstructured Workload Primitive", Label("unstructured-workload"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-uns-wl-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation and Health", func() {
		It("should create an unstructured ConfigMap and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cm := newUnstructuredConfigMap(ns, "uns-wl-config", map[string]string{
					"setting": "enabled",
				})
				return workload.NewBuilder(cm).
					WithCustomConvergeStatus(unstructuredAlwaysHealthy).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ConfigMap exists with correct data")
			got := &uns.Unstructured{}
			got.SetGroupVersionKind(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"})
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "uns-wl-config", Namespace: ns}, got)).To(Succeed())
			gotData, _, _ := uns.NestedStringMap(got.Object, "data")
			Expect(gotData).To(HaveKeyWithValue("setting", "enabled"))

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

	Context("Grace Period — Degraded", func() {
		It("should report Degraded when the converging handler never converges", func() {
			gracePeriod := 5 * time.Second

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				cm := newUnstructuredConfigMap(ns, "uns-wl-degraded", map[string]string{
					"state": "pending",
				})

				res, err := workload.NewBuilder(cm).
					WithCustomConvergeStatus(unstructuredAlwaysCreating).
					WithCustomGraceStatus(unstructuredGraceDegraded).
					Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("e2e-uns-degraded").
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
				cm := newUnstructuredConfigMap(ns, "uns-wl-error", nil)
				return workload.NewBuilder(cm).
					WithCustomConvergeStatus(unstructuredAlwaysHealthy).
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
				cm := newUnstructuredConfigMap(ns, "uns-wl-guarded", map[string]string{"key": "value"})
				return workload.NewBuilder(cm).
					WithCustomConvergeStatus(unstructuredAlwaysHealthy).
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
