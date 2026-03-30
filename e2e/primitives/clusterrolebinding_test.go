//go:build e2e

package primitives

import (
	"fmt"
	"sync/atomic"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrolebinding"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseClusterRoleBinding(name, roleRefName, saName, saNamespace string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     roleRefName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      saName,
				Namespace: saNamespace,
			},
		},
	}
}

var _ = Describe("ClusterRoleBinding Primitive", Label("clusterrolebinding"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-clusterrolebinding-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a ClusterRoleBinding and reach Healthy condition", func() {
			crbName := "e2e-crb-create-" + ns

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				crb := newBaseClusterRoleBinding(crbName, "view", "default", ns)
				return clusterrolebinding.NewBuilder(crb).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ClusterRoleBinding exists with correct role ref and subjects")
			var crb rbacv1.ClusterRoleBinding
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crbName}, &crb)).To(Succeed())
			Expect(crb.RoleRef.APIGroup).To(Equal("rbac.authorization.k8s.io"))
			Expect(crb.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(crb.RoleRef.Name).To(Equal("view"))
			Expect(crb.Subjects).To(HaveLen(1))
			Expect(crb.Subjects[0].Kind).To(Equal("ServiceAccount"))
			Expect(crb.Subjects[0].Name).To(Equal("default"))
			Expect(crb.Subjects[0].Namespace).To(Equal(ns))

			By("verifying owner reference is set")
			expectOwnerReference(crb.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply subject mutations to the ClusterRoleBinding", func() {
			crbName := "e2e-crb-mutate-" + ns

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				crb := newBaseClusterRoleBinding(crbName, "view", "default", ns)
				return clusterrolebinding.NewBuilder(crb).
					WithMutation(clusterrolebinding.Mutation{
						Name: "add-extra-subject",
						Mutate: func(m *clusterrolebinding.Mutator) error {
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
			var crb rbacv1.ClusterRoleBinding
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crbName}, &crb)).To(Succeed())
			Expect(crb.Subjects).To(HaveLen(2))
			Expect(crb.Subjects).To(ContainElement(rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: ns,
			}))
			Expect(crb.Subjects).To(ContainElement(rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      "extra-sa",
				Namespace: ns,
			}))
		})
	})

	Context("Updates", func() {
		It("should propagate subject changes on re-reconciliation", func() {
			crbName := "e2e-crb-update-" + ns
			var useUpdatedSubjects atomic.Bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				saName := "default"
				if useUpdatedSubjects.Load() {
					saName = "updated-sa"
				}
				crb := newBaseClusterRoleBinding(crbName, "view", saName, ns)
				return clusterrolebinding.NewBuilder(crb).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial subjects")
			var crb rbacv1.ClusterRoleBinding
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crbName}, &crb)).To(Succeed())
			Expect(crb.Subjects).To(HaveLen(1))
			Expect(crb.Subjects[0].Name).To(Equal("default"))

			By("switching desired subjects and triggering reconciliation")
			useUpdatedSubjects.Store(true)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-subjects"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated subjects")
			Eventually(func(g Gomega) string {
				var updated rbacv1.ClusterRoleBinding
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crbName}, &updated)).To(Succeed())
				g.Expect(updated.Subjects).To(HaveLen(1))
				return updated.Subjects[0].Name
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("updated-sa"))
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			crbName := "e2e-crb-error-" + ns

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				crb := newBaseClusterRoleBinding(crbName, "view", "default", ns)
				return clusterrolebinding.NewBuilder(crb).
					WithMutation(clusterrolebinding.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *clusterrolebinding.Mutator) error {
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
			crbName := "e2e-crb-guard-" + ns

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				crb := newBaseClusterRoleBinding(crbName, "view", "default", ns)
				return clusterrolebinding.NewBuilder(crb).
					WithGuard(func(_ rbacv1.ClusterRoleBinding) (concepts.GuardStatusWithReason, error) {
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
