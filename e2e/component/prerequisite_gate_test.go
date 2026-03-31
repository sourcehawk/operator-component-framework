//go:build e2e

package component

import (
	"sync/atomic"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/configmap"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

// atomicGate is a feature.Gate backed by an atomic boolean for concurrent test use.
type atomicGate struct {
	enabled atomic.Bool
}

func (g *atomicGate) Enabled() (bool, error) {
	return g.enabled.Load(), nil
}

// atomicPrerequisite is a component.Prerequisite backed by an atomic boolean.
// It also counts how many times Check was called.
type atomicPrerequisite struct {
	met       atomic.Bool
	callCount atomic.Int32
	reason    string
}

func (p *atomicPrerequisite) Check(_ component.ReconcileContext) (component.PrerequisiteResult, error) {
	p.callCount.Add(1)
	if p.met.Load() {
		return component.PrerequisiteResult{Status: component.PrerequisiteStatusMet}, nil
	}
	return component.PrerequisiteResult{
		Status: component.PrerequisiteStatusNotMet,
		Reason: p.reason,
	}, nil
}

var _ = Describe("Component Feature Gate and Prerequisite", func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-gate-prereq-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Feature Gate", func() {
		It("should delete all resources when gate is disabled", func() {
			gate := &atomicGate{}
			gate.enabled.Store(true)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				depRes, err := deployment.NewBuilder(newDeployment(ns, "gated-web", 1)).Build()
				if err != nil {
					return nil, err
				}

				cmRes, err := configmap.NewBuilder(newConfigMap(ns, "gated-config", map[string]string{
					"key": "value",
				})).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("gate-test").
					WithConditionType("E2EReady").
					WithFeatureGate(gate).
					WithResource(depRes, component.ResourceOptions{}).
					WithResource(cmRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Healthy state with gate enabled")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying both resources exist")
			var dep appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "gated-web", Namespace: ns}, &dep)).To(Succeed())
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "gated-config", Namespace: ns}, &cm)).To(Succeed())

			By("disabling the gate and triggering reconcile")
			gate.enabled.Store(false)
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				if a.Annotations == nil {
					a.Annotations = map[string]string{}
				}
				a.Annotations["test.ocf.io/trigger"] = "disable-gate"
			})

			By("waiting for Disabled condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Disabled"))

			By("verifying resources are deleted")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "gated-web", Namespace: ns}, &dep)
			}, framework.DefaultTimeout, framework.DefaultPolling).ShouldNot(Succeed())
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "gated-config", Namespace: ns}, &cm)
			}, framework.DefaultTimeout, framework.DefaultPolling).ShouldNot(Succeed())
		})

		It("should create resources when gate becomes enabled", func() {
			gate := &atomicGate{}
			gate.enabled.Store(false)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				cmRes, err := configmap.NewBuilder(newConfigMap(ns, "late-config", map[string]string{
					"key": "value",
				})).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("gate-enable-test").
					WithConditionType("E2EReady").
					WithFeatureGate(gate).
					WithResource(cmRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Disabled condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Disabled"))

			By("verifying no resources exist")
			var cm corev1.ConfigMap
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "late-config", Namespace: ns}, &cm)
			Expect(err).To(HaveOccurred())

			By("enabling the gate and triggering reconcile")
			gate.enabled.Store(true)
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				if a.Annotations == nil {
					a.Annotations = map[string]string{}
				}
				a.Annotations["test.ocf.io/trigger"] = "enable-gate"
			})

			By("waiting for Healthy condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying resource was created")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "late-config", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("key", "value"))
		})

		It("should take precedence over suspension when gate is disabled", func() {
			gate := &atomicGate{}
			gate.enabled.Store(false)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				cmRes, err := configmap.NewBuilder(newConfigMap(ns, "gate-sus-cm", map[string]string{
					"key": "value",
				})).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("gate-sus-test").
					WithConditionType("E2EReady").
					WithFeatureGate(gate).
					WithResource(cmRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)
			app.Spec.Suspended = true
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Disabled condition (not Suspended)")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Disabled"))
		})
	})

	Context("Prerequisite", func() {
		It("should block reconciliation until prerequisite is met", func() {
			prereq := &atomicPrerequisite{reason: "dependency not ready"}
			prereq.met.Store(false)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				cmRes, err := configmap.NewBuilder(newConfigMap(ns, "prereq-cm", map[string]string{
					"key": "value",
				})).Build()
				if err != nil {
					return nil, err
				}

				depRes, err := deployment.NewBuilder(newDeployment(ns, "prereq-web", 1)).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("prereq-test").
					WithConditionType("E2EReady").
					WithPrerequisite(prereq).
					WithResource(cmRes, component.ResourceOptions{}).
					WithResource(depRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for PrerequisiteNotMet condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "PrerequisiteNotMet"))

			By("verifying no resources were created")
			var cm corev1.ConfigMap
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "prereq-cm", Namespace: ns}, &cm)
			Expect(err).To(HaveOccurred())
			var dep appsv1.Deployment
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "prereq-web", Namespace: ns}, &dep)
			Expect(err).To(HaveOccurred())

			By("satisfying the prerequisite and triggering reconcile")
			prereq.met.Store(true)
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				if a.Annotations == nil {
					a.Annotations = map[string]string{}
				}
				a.Annotations["test.ocf.io/trigger"] = "unblock-prereq"
			})

			By("waiting for Healthy condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying resources were created")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "prereq-cm", Namespace: ns}, &cm)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "prereq-web", Namespace: ns}, &dep)).To(Succeed())
		})

		It("should never re-evaluate prerequisite after barrier passes", func() {
			prereq := &atomicPrerequisite{reason: "should not block"}
			prereq.met.Store(true)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				cmRes, err := configmap.NewBuilder(newConfigMap(ns, "barrier-cm", map[string]string{
					"key": "value",
				})).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("barrier-test").
					WithConditionType("E2EReady").
					WithPrerequisite(prereq).
					WithResource(cmRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Healthy condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("recording call count after first successful reconcile")
			countAfterFirst := prereq.callCount.Load()

			By("making prerequisite not-met and triggering reconcile")
			prereq.met.Store(false)
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				if a.Annotations == nil {
					a.Annotations = map[string]string{}
				}
				a.Annotations["test.ocf.io/trigger"] = "check-barrier"
			})

			By("verifying condition stays Healthy (prerequisite not re-evaluated)")
			Consistently(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), "5s", framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying call count did not increase")
			Expect(prereq.callCount.Load()).To(Equal(countAfterFirst))
		})

		It("should block suspension while prerequisite has not passed", func() {
			prereq := &atomicPrerequisite{reason: "dependency not ready"}
			prereq.met.Store(false)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				depRes, err := deployment.NewBuilder(newDeployment(ns, "sus-prereq-web", 1)).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("sus-prereq-test").
					WithConditionType("E2EReady").
					WithPrerequisite(prereq).
					WithResource(depRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for PrerequisiteNotMet condition before suspending")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "PrerequisiteNotMet"))

			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				a.Spec.Suspended = true
			})

			By("verifying condition remains PrerequisiteNotMet (not Suspended)")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "PrerequisiteNotMet"))

			By("verifying no resources were created")
			var dep appsv1.Deployment
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "sus-prereq-web", Namespace: ns}, &dep)
			Expect(err).To(HaveOccurred())
		})

		It("should allow suspension after prerequisite has passed", func() {
			prereq := &atomicPrerequisite{reason: "dependency not ready"}
			prereq.met.Store(true)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				depRes, err := deployment.NewBuilder(newDeployment(ns, "prereq-sus-web", 1)).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("prereq-sus-test").
					WithConditionType("E2EReady").
					WithPrerequisite(prereq).
					WithResource(depRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Healthy condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("suspending the component")
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				a.Spec.Suspended = true
			})

			By("waiting for Suspended condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Suspended"))
		})
	})
})
