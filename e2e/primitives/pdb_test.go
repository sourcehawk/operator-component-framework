//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pdb"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBasePDB(namespace, name string) *policyv1.PodDisruptionBudget {
	minAvailable := intstr.FromInt32(1)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailable,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
		},
	}
}

var _ = Describe("pdb Primitive", Label("pdb"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-pdb-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a PodDisruptionBudget and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePDB(ns, "app-pdb")
				return pdb.NewBuilder(obj).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the PodDisruptionBudget exists with correct spec")
			var p policyv1.PodDisruptionBudget
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-pdb", Namespace: ns}, &p)).To(Succeed())
			Expect(p.Spec.MinAvailable).NotTo(BeNil())
			Expect(p.Spec.MinAvailable.IntValue()).To(Equal(1))
			Expect(p.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app", "app-pdb"))

			By("verifying owner reference is set")
			Expect(p.OwnerReferences).NotTo(BeEmpty())
			Expect(p.OwnerReferences[0].Kind).To(Equal("ClusterTestApp"))
			Expect(p.OwnerReferences[0].Name).To(Equal(name))
		})
	})

	Context("Mutations", func() {
		It("should apply spec mutations to the PodDisruptionBudget", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePDB(ns, "app-pdb-mutated")
				return pdb.NewBuilder(obj).
					WithMutation(pdb.Mutation{
						Name: "set-max-unavailable",
						Mutate: func(m *pdb.Mutator) error {
							m.EditSpec(func(e *editors.PodDisruptionBudgetSpecEditor) error {
								e.ClearMinAvailable()
								e.SetMaxUnavailable(intstr.FromString("25%"))
								return nil
							})
							return nil
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the mutation was applied")
			var p policyv1.PodDisruptionBudget
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-pdb-mutated", Namespace: ns}, &p)).To(Succeed())
			Expect(p.Spec.MinAvailable).To(BeNil())
			Expect(p.Spec.MaxUnavailable).NotTo(BeNil())
			Expect(p.Spec.MaxUnavailable.String()).To(Equal("25%"))
		})
	})

	Context("Updates", func() {
		It("should propagate spec changes on re-reconciliation", func() {
			var useUpdatedSpec bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePDB(ns, "app-pdb-update")
				if useUpdatedSpec {
					maxUnavailable := intstr.FromInt32(2)
					obj.Spec.MinAvailable = nil
					obj.Spec.MaxUnavailable = &maxUnavailable
				}
				return pdb.NewBuilder(obj).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial spec")
			var p policyv1.PodDisruptionBudget
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-pdb-update", Namespace: ns}, &p)).To(Succeed())
			Expect(p.Spec.MinAvailable).NotTo(BeNil())
			Expect(p.Spec.MinAvailable.IntValue()).To(Equal(1))

			By("switching spec and triggering reconciliation")
			useUpdatedSpec = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-spec"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated spec")
			Eventually(func(g Gomega) {
				var updated policyv1.PodDisruptionBudget
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-pdb-update", Namespace: ns}, &updated)).To(Succeed())
				g.Expect(updated.Spec.MinAvailable).To(BeNil())
				g.Expect(updated.Spec.MaxUnavailable).NotTo(BeNil())
				g.Expect(updated.Spec.MaxUnavailable.IntValue()).To(Equal(2))
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePDB(ns, "app-pdb-error")
				return pdb.NewBuilder(obj).
					WithMutation(pdb.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *pdb.Mutator) error {
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
