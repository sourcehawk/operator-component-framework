//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/rolebinding"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseRoleBinding(namespace, name string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "test-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: namespace,
			},
		},
	}
}

var _ = Describe("rolebinding Primitive", Label("rolebinding"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-rolebinding-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a RoleBinding and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				rb := newBaseRoleBinding(ns, "test-rb")
				return rolebinding.NewBuilder(rb).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the RoleBinding exists with correct spec")
			var rb rbacv1.RoleBinding
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-rb", Namespace: ns}, &rb)).To(Succeed())
			Expect(rb.RoleRef.Kind).To(Equal("Role"))
			Expect(rb.RoleRef.Name).To(Equal("test-role"))
			Expect(rb.RoleRef.APIGroup).To(Equal("rbac.authorization.k8s.io"))
			Expect(rb.Subjects).To(HaveLen(1))
			Expect(rb.Subjects[0].Kind).To(Equal("ServiceAccount"))
			Expect(rb.Subjects[0].Name).To(Equal("default"))

			By("verifying owner reference is set")
			expectOwnerReference(rb.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply subject mutations to the RoleBinding", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				rb := newBaseRoleBinding(ns, "test-rb-mutated")
				return rolebinding.NewBuilder(rb).
					WithMutation(rolebinding.Mutation{
						Name: "add-extra-subject",
						Mutate: func(m *rolebinding.Mutator) error {
							m.EditSubjects(func(e *editors.BindingSubjectsEditor) error {
								e.EnsureServiceAccount("extra-sa", ns)
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

			By("verifying both base and mutated subjects are present")
			var rb rbacv1.RoleBinding
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-rb-mutated", Namespace: ns}, &rb)).To(Succeed())
			Expect(rb.Subjects).To(HaveLen(2))
			Expect(rb.Subjects).To(ContainElement(rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: ns,
			}))
			Expect(rb.Subjects).To(ContainElement(rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      "extra-sa",
				Namespace: ns,
			}))
		})
	})

	Context("Updates", func() {
		It("should propagate subject changes on re-reconciliation", func() {
			var useUpdatedSubjects bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				rb := newBaseRoleBinding(ns, "test-rb-update")
				if useUpdatedSubjects {
					rb.Subjects = []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "updated-sa",
							Namespace: ns,
						},
					}
				}
				return rolebinding.NewBuilder(rb).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial subjects")
			var rb rbacv1.RoleBinding
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-rb-update", Namespace: ns}, &rb)).To(Succeed())
			Expect(rb.Subjects).To(HaveLen(1))
			Expect(rb.Subjects[0].Name).To(Equal("default"))

			By("switching desired subjects and triggering reconciliation")
			useUpdatedSubjects = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-subjects"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated subjects")
			Eventually(func(g Gomega) string {
				var updated rbacv1.RoleBinding
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "test-rb-update", Namespace: ns}, &updated)).To(Succeed())
				g.Expect(updated.Subjects).To(HaveLen(1))
				return updated.Subjects[0].Name
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("updated-sa"))
		})
	})

	Context("Error", func() {
		It("should report Error condition when mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				rb := newBaseRoleBinding(ns, "test-rb-error")
				return rolebinding.NewBuilder(rb).
					WithMutation(rolebinding.Mutation{
						Name: "intentional-failure",
						Mutate: func(m *rolebinding.Mutator) error {
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
