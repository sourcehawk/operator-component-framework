//go:build e2e

package primitives

import (
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/daemonset"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseDaemonSet(namespace, name string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app",
							Image: "nginx:1.27",
						},
					},
				},
			},
		},
	}
}

var _ = Describe("DaemonSet Primitive", Label("daemonset"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-daemonset-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation and Health", func() {
		It("should create a DaemonSet and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ds := newBaseDaemonSet(ns, "ds-create")
				return daemonset.NewBuilder(ds).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the DaemonSet exists with correct spec")
			var ds appsv1.DaemonSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ds-create", Namespace: ns}, &ds)).To(Succeed())
			Expect(ds.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.27"))

			By("verifying owner reference is set")
			expectOwnerReference(ds.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply feature mutations to the DaemonSet", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ds := newBaseDaemonSet(ns, "ds-mutated")
				return daemonset.NewBuilder(ds).
					WithMutation(daemonset.Mutation{
						Name: "add-label",
						Mutate: func(m *daemonset.Mutator) error {
							m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
								e.EnsureLabel("e2e-mutation", "applied")
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

			By("verifying the mutation label is present on the pod template")
			var ds appsv1.DaemonSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ds-mutated", Namespace: ns}, &ds)).To(Succeed())
			Expect(ds.Spec.Template.Labels).To(HaveKeyWithValue("e2e-mutation", "applied"))
		})
	})

	Context("Updates", func() {
		It("should propagate image changes on re-reconciliation", func() {
			var useUpdatedImage bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ds := newBaseDaemonSet(ns, "ds-update")
				if useUpdatedImage {
					ds.Spec.Template.Spec.Containers[0].Image = "nginx:1.26"
				}
				return daemonset.NewBuilder(ds).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("switching the desired image and triggering reconciliation")
			useUpdatedImage = true
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				if a.Annotations == nil {
					a.Annotations = map[string]string{}
				}
				a.Annotations["e2e.ocf.io/trigger"] = "update-image"
			})

			By("verifying the DaemonSet image is updated")
			Eventually(func(g Gomega) string {
				var ds appsv1.DaemonSet
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ds-update", Namespace: ns}, &ds)).To(Succeed())
				return ds.Spec.Template.Spec.Containers[0].Image
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("nginx:1.26"))

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))
		})
	})

	Context("Suspension", func() {
		It("should delete the DaemonSet when suspended and recreate when un-suspended", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ds := newBaseDaemonSet(ns, "ds-suspend")
				return daemonset.NewBuilder(ds).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("suspending the ClusterTestApp")
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				a.Spec.Suspended = true
			})

			By("waiting for Suspended condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Suspended"))

			By("verifying the DaemonSet is deleted")
			Eventually(func() bool {
				var ds appsv1.DaemonSet
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "ds-suspend", Namespace: ns}, &ds)
				return apierrors.IsNotFound(err)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(BeTrue())

			By("un-suspending the ClusterTestApp")
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				a.Spec.Suspended = false
			})

			By("waiting for Healthy state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the DaemonSet is recreated")
			var ds appsv1.DaemonSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "ds-suspend", Namespace: ns}, &ds)).To(Succeed())
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded when not fully converged but pods are ready after grace period expires", func() {
			gracePeriod := 5 * time.Second

			// On a single-node kind cluster, a DaemonSet's DesiredNumberScheduled=1.
			// When NumberReady=1, the default grace handler reports Healthy because
			// all desired pods are ready. To test the Degraded grace path we use a
			// custom converging handler that always reports "Scaling" (so the grace
			// handler is invoked after the period expires) and a custom grace handler
			// that returns Degraded.
			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				ds := newBaseDaemonSet(ns, "ds-degraded")

				res, err := daemonset.NewBuilder(ds).
					WithCustomConvergeStatus(func(_ concepts.ConvergingOperation, _ *appsv1.DaemonSet) (concepts.AliveStatusWithReason, error) {
						return concepts.AliveStatusWithReason{
							Status: concepts.AliveConvergingStatusScaling,
							Reason: "Forced non-converged for e2e test",
						}, nil
					}).
					WithCustomGraceStatus(func(_ *appsv1.DaemonSet) (concepts.GraceStatusWithReason, error) {
						return concepts.GraceStatusWithReason{
							Status: concepts.GraceStatusDegraded,
							Reason: "Forced degraded for e2e test",
						}, nil
					}).
					Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("e2e-degraded").
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

	Context("Grace Period — Down", func() {
		It("should report Down when no pods are ready after grace period expires", func() {
			gracePeriod := 5 * time.Second

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				ds := newBaseDaemonSet(ns, "ds-down")
				ds.Spec.Template.Spec.Containers[0].Image = "does-not-exist:e2e-test"

				res, err := daemonset.NewBuilder(ds).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("e2e-down").
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

			By("waiting for Down condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Down"))
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ds := newBaseDaemonSet(ns, "ds-error")
				return daemonset.NewBuilder(ds).
					WithMutation(daemonset.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *daemonset.Mutator) error {
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
				ds := newBaseDaemonSet(ns, "guarded-ds")
				return daemonset.NewBuilder(ds).
					WithGuard(func(_ appsv1.DaemonSet) (concepts.GuardStatusWithReason, error) {
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
