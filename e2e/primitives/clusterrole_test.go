//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrole"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseClusterRole(name string, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Rules: rules,
	}
}

var _ = Describe("ClusterRole Primitive", Label("ClusterRole"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-clusterrole-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a ClusterRole and reach Healthy condition", func() {
			crName := "e2e-cr-create-" + ns

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cr := newBaseClusterRole(crName, []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"configmaps"},
						Verbs:     []string{"get", "list", "watch"},
					},
				})
				return clusterrole.NewBuilder(cr).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ClusterRole exists with correct rules")
			var cr rbacv1.ClusterRole
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName}, &cr)).To(Succeed())
			Expect(cr.Rules).To(HaveLen(1))
			Expect(cr.Rules[0].APIGroups).To(ContainElement(""))
			Expect(cr.Rules[0].Resources).To(ContainElement("configmaps"))
			Expect(cr.Rules[0].Verbs).To(ConsistOf("get", "list", "watch"))

			By("verifying owner reference is set")
			Expect(cr.OwnerReferences).NotTo(BeEmpty())
			Expect(cr.OwnerReferences[0].Kind).To(Equal("ClusterTestApp"))
			Expect(cr.OwnerReferences[0].Name).To(Equal(name))
		})
	})

	Context("Mutations", func() {
		It("should apply rule mutations to the ClusterRole", func() {
			crName := "e2e-cr-mutate-" + ns

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cr := newBaseClusterRole(crName, []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get"},
					},
				})
				return clusterrole.NewBuilder(cr).
					WithMutation(clusterrole.Mutation{
						Name: "add-secrets-rule",
						Mutate: func(m *clusterrole.Mutator) error {
							m.AddRule(rbacv1.PolicyRule{
								APIGroups: []string{""},
								Resources: []string{"secrets"},
								Verbs:     []string{"get", "list"},
							})
							return nil
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying both base and mutated rules are present")
			var cr rbacv1.ClusterRole
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName}, &cr)).To(Succeed())
			Expect(cr.Rules).To(HaveLen(2))
			Expect(cr.Rules).To(ContainElement(SatisfyAll(
				HaveField("Resources", ContainElement("pods")),
			)))
			Expect(cr.Rules).To(ContainElement(SatisfyAll(
				HaveField("Resources", ContainElement("secrets")),
				HaveField("Verbs", ConsistOf("get", "list")),
			)))
		})
	})

	Context("Updates", func() {
		It("should propagate rule changes on re-reconciliation", func() {
			crName := "e2e-cr-update-" + ns
			var useUpdatedRules bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				rules := []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"configmaps"},
						Verbs:     []string{"get"},
					},
				}
				if useUpdatedRules {
					rules = []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"configmaps", "secrets"},
							Verbs:     []string{"get", "list", "watch"},
						},
					}
				}
				cr := newBaseClusterRole(crName, rules)
				return clusterrole.NewBuilder(cr).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial rules")
			var cr rbacv1.ClusterRole
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName}, &cr)).To(Succeed())
			Expect(cr.Rules).To(HaveLen(1))
			Expect(cr.Rules[0].Verbs).To(ConsistOf("get"))

			By("switching desired rules and triggering reconciliation")
			useUpdatedRules = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-rules"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated rules")
			Eventually(func(g Gomega) []rbacv1.PolicyRule {
				var updated rbacv1.ClusterRole
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName}, &updated)).To(Succeed())
				return updated.Rules
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(And(
				HaveLen(1),
				ContainElement(SatisfyAll(
					HaveField("Resources", ConsistOf("configmaps", "secrets")),
					HaveField("Verbs", ConsistOf("get", "list", "watch")),
				)),
			))
		})
	})

	Context("Error", func() {
		It("should report Error condition when mutation fails", func() {
			crName := "e2e-cr-error-" + ns

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cr := newBaseClusterRole(crName, []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get"},
					},
				})
				return clusterrole.NewBuilder(cr).
					WithMutation(clusterrole.Mutation{
						Name: "intentional-failure",
						Mutate: func(m *clusterrole.Mutator) error {
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
