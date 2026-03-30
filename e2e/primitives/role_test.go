//go:build e2e

package primitives

import (
	"fmt"
	"sync/atomic"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/role"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseRole(namespace, name string, rules []rbacv1.PolicyRule) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Rules: rules,
	}
}

var _ = Describe("Role Primitive", Label("role"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-role-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a Role and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				r := newBaseRole(ns, "app-role", []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get", "list"},
					},
				})
				return role.NewBuilder(r).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the Role exists with correct rules")
			var r rbacv1.Role
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-role", Namespace: ns}, &r)).To(Succeed())
			Expect(r.Rules).To(HaveLen(1))
			Expect(r.Rules[0].APIGroups).To(Equal([]string{""}))
			Expect(r.Rules[0].Resources).To(Equal([]string{"pods"}))
			Expect(r.Rules[0].Verbs).To(Equal([]string{"get", "list"}))

			By("verifying owner reference is set")
			expectOwnerReference(r.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply rule mutations to the Role", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				r := newBaseRole(ns, "app-role-mutated", []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"configmaps"},
						Verbs:     []string{"get"},
					},
				})
				return role.NewBuilder(r).
					WithMutation(role.Mutation{
						Name: "add-extra-rule",
						Mutate: func(m *role.Mutator) error {
							m.EditRules(func(e *editors.PolicyRulesEditor) error {
								e.AddRule(rbacv1.PolicyRule{
									APIGroups: []string{""},
									Resources: []string{"secrets"},
									Verbs:     []string{"get", "list"},
								})
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

			By("verifying both base and mutated rules are present")
			var r rbacv1.Role
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-role-mutated", Namespace: ns}, &r)).To(Succeed())
			Expect(r.Rules).To(HaveLen(2))

			Expect(r.Rules).To(ConsistOf(
				Equal(rbacv1.PolicyRule{
					APIGroups: []string{""},
					Resources: []string{"configmaps"},
					Verbs:     []string{"get"},
				}),
				Equal(rbacv1.PolicyRule{
					APIGroups: []string{""},
					Resources: []string{"secrets"},
					Verbs:     []string{"get", "list"},
				}),
			))
		})
	})

	Context("Updates", func() {
		It("should propagate rule changes on re-reconciliation", func() {
			var useUpdatedRules atomic.Bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				rules := []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get"},
					},
				}
				if useUpdatedRules.Load() {
					rules = []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"pods"},
							Verbs:     []string{"get", "list", "watch"},
						},
						{
							APIGroups: []string{"apps"},
							Resources: []string{"deployments"},
							Verbs:     []string{"get"},
						},
					}
				}
				r := newBaseRole(ns, "app-role-update", rules)
				return role.NewBuilder(r).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial rules")
			var r rbacv1.Role
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-role-update", Namespace: ns}, &r)).To(Succeed())
			Expect(r.Rules).To(HaveLen(1))
			Expect(r.Rules[0].Verbs).To(Equal([]string{"get"}))

			By("switching desired rules and triggering reconciliation")
			useUpdatedRules.Store(true)
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				if a.Annotations == nil {
					a.Annotations = map[string]string{}
				}
				a.Annotations["e2e.ocf.io/trigger"] = "update-rules"
			})

			By("verifying updated rules")
			Eventually(func(g Gomega) {
				var updated rbacv1.Role
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-role-update", Namespace: ns}, &updated)).To(Succeed())
				g.Expect(updated.Rules).To(HaveLen(2))

				g.Expect(updated.Rules).To(ConsistOf(
					Equal(rbacv1.PolicyRule{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get", "list", "watch"},
					}),
					Equal(rbacv1.PolicyRule{
						APIGroups: []string{"apps"},
						Resources: []string{"deployments"},
						Verbs:     []string{"get"},
					}),
				))
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				r := newBaseRole(ns, "app-role-error", []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get"},
					},
				})
				return role.NewBuilder(r).
					WithMutation(role.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *role.Mutator) error {
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
				r := newBaseRole(ns, "guarded-role", []rbacv1.PolicyRule{
					{
						APIGroups: []string{""},
						Resources: []string{"pods"},
						Verbs:     []string{"get"},
					},
				})
				return role.NewBuilder(r).
					WithGuard(func(_ rbacv1.Role) (concepts.GuardStatusWithReason, error) {
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
