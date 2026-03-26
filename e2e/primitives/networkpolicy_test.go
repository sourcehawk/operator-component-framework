//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/networkpolicy"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseNetworkPolicy(namespace, name string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{}, // allow all ingress
			},
		},
	}
}

var _ = Describe("networkpolicy Primitive", Label("networkpolicy"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-networkpolicy-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a NetworkPolicy and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				np := newBaseNetworkPolicy(ns, "allow-all-ingress")
				return networkpolicy.NewBuilder(np).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the NetworkPolicy exists with correct spec")
			var np networkingv1.NetworkPolicy
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "allow-all-ingress", Namespace: ns}, &np)).To(Succeed())
			Expect(np.Spec.PolicyTypes).To(ContainElement(networkingv1.PolicyTypeIngress))
			Expect(np.Spec.Ingress).To(HaveLen(1))

			By("verifying owner reference is set")
			Expect(np.OwnerReferences).NotTo(BeEmpty())
			Expect(np.OwnerReferences[0].Kind).To(Equal("ClusterTestApp"))
			Expect(np.OwnerReferences[0].Name).To(Equal(name))
		})
	})

	Context("Mutations", func() {
		It("should apply spec mutations to the NetworkPolicy", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				np := newBaseNetworkPolicy(ns, "mutated-policy")
				return networkpolicy.NewBuilder(np).
					WithMutation(networkpolicy.Mutation{
						Name: "add-egress-rule",
						Mutate: func(m *networkpolicy.Mutator) error {
							m.EditNetworkPolicySpec(func(e *editors.NetworkPolicySpecEditor) error {
								e.SetPolicyTypes([]networkingv1.PolicyType{
									networkingv1.PolicyTypeIngress,
									networkingv1.PolicyTypeEgress,
								})
								e.AppendEgressRule(networkingv1.NetworkPolicyEgressRule{})
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

			By("verifying both ingress and egress policy types are present")
			var np networkingv1.NetworkPolicy
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "mutated-policy", Namespace: ns}, &np)).To(Succeed())
			Expect(np.Spec.PolicyTypes).To(ContainElements(networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress))
			Expect(np.Spec.Egress).To(HaveLen(1))
		})
	})

	Context("Updates", func() {
		It("should propagate spec changes on re-reconciliation", func() {
			var useUpdatedSpec bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				np := newBaseNetworkPolicy(ns, "updatable-policy")
				if useUpdatedSpec {
					np.Spec.PodSelector = metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "updated"},
					}
					np.Spec.PolicyTypes = []networkingv1.PolicyType{
						networkingv1.PolicyTypeIngress,
						networkingv1.PolicyTypeEgress,
					}
					np.Spec.Egress = []networkingv1.NetworkPolicyEgressRule{{}}
				}
				return networkpolicy.NewBuilder(np).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial spec")
			var np networkingv1.NetworkPolicy
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "updatable-policy", Namespace: ns}, &np)).To(Succeed())
			Expect(np.Spec.PodSelector.MatchLabels).To(BeEmpty())

			By("switching desired spec and triggering reconciliation")
			useUpdatedSpec = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-spec"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated spec")
			Eventually(func(g Gomega) map[string]string {
				var updated networkingv1.NetworkPolicy
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "updatable-policy", Namespace: ns}, &updated)).To(Succeed())
				return updated.Spec.PodSelector.MatchLabels
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(HaveKeyWithValue("app", "updated"))
		})
	})

	Context("Error", func() {
		It("should reach Error condition when mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				np := newBaseNetworkPolicy(ns, "error-policy")
				return networkpolicy.NewBuilder(np).
					WithMutation(networkpolicy.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *networkpolicy.Mutator) error {
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
