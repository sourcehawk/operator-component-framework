//go:build e2e

package primitives

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pod"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBasePod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "nginx:1.27",
				},
			},
		},
	}
}

var _ = Describe("Pod Primitive", Label("pod"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-pod-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation and Health", func() {
		It("should create a Pod and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				p := newBasePod(ns, "web-server")
				return pod.NewBuilder(p).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the Pod exists with correct spec")
			var p corev1.Pod
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-server", Namespace: ns}, &p)).To(Succeed())
			Expect(p.Spec.Containers[0].Image).To(Equal("nginx:1.27"))
			Expect(p.Spec.Containers[0].Name).To(Equal("app"))

			By("verifying owner reference is set")
			Expect(p.OwnerReferences).NotTo(BeEmpty())
			var ownerRefFound bool
			for _, or := range p.OwnerReferences {
				if or.Kind == "ClusterTestApp" && or.Name == name {
					ownerRefFound = true
					break
				}
			}
			Expect(ownerRefFound).To(BeTrue(), fmt.Sprintf("expected owner reference with Kind=ClusterTestApp and Name=%s", name))
		})
	})

	Context("Mutations", func() {
		It("should apply feature mutations to the Pod", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				p := newBasePod(ns, "web-mutated")
				return pod.NewBuilder(p).
					WithMutation(pod.Mutation{
						Name: "add-env",
						Mutate: func(m *pod.Mutator) error {
							m.EditContainers(selectors.AllContainers(), func(e *editors.ContainerEditor) error {
								e.EnsureEnvVar(corev1.EnvVar{Name: "E2E_TEST", Value: "true"})
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

			By("verifying the env var is present on the Pod")
			var p corev1.Pod
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-mutated", Namespace: ns}, &p)).To(Succeed())

			envVars := p.Spec.Containers[0].Env
			var found bool
			for _, ev := range envVars {
				if ev.Name == "E2E_TEST" && ev.Value == "true" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "expected E2E_TEST env var on container")
		})
	})

	Context("Suspension", func() {
		It("should delete the Pod when suspended and recreate when un-suspended", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				p := newBasePod(ns, "web-suspend")
				return pod.NewBuilder(p).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("suspending the ClusterTestApp")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = true
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Suspended condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Suspended"))

			By("verifying the Pod is deleted")
			Eventually(func() bool {
				var p corev1.Pod
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "web-suspend", Namespace: ns}, &p)
				return apierrors.IsNotFound(err)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(BeTrue(), "expected Pod to be deleted (NotFound)")

			By("un-suspending the ClusterTestApp")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = false
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Healthy state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the Pod is recreated")
			var p corev1.Pod
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-suspend", Namespace: ns}, &p)).To(Succeed())
		})
	})

	Context("Updates", func() {
		It("should propagate image changes on re-reconciliation", func() {
			var useUpdatedImage atomic.Bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				image := "nginx:1.27"
				if useUpdatedImage.Load() {
					image = "nginx:1.26"
				}
				p := newBasePod(ns, "web-update")
				p.Spec.Containers[0].Image = image
				return pod.NewBuilder(p).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("switching the desired image and triggering reconciliation")
			useUpdatedImage.Store(true)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-image"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying the Pod image is updated")
			Eventually(func(g Gomega) string {
				var p corev1.Pod
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-update", Namespace: ns}, &p)).To(Succeed())
				return p.Spec.Containers[0].Image
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("nginx:1.26"))

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded when Pod is running but not all containers are ready after grace period expires", func() {
			gracePeriod := 5 * time.Second

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				p := newBasePod(ns, "web-degraded")
				// Add a second container with a readiness probe that always fails,
				// so the pod will be Running but not all containers ready -> Degraded.
				p.Spec.Containers = append(p.Spec.Containers, corev1.Container{
					Name:  "sidecar",
					Image: "nginx:1.27",
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"false"},
							},
						},
						PeriodSeconds:    1,
						FailureThreshold: 1,
					},
				})

				res, err := pod.NewBuilder(p).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("e2e-degraded").
					WithConditionType("E2EReady").
					WithResource(res, component.ResourceOptions{}).
					WithGracePeriod(gracePeriod).
					Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for component condition to be created")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				ShouldNot(BeNil())

			By("waiting for the Pod to be running before starting grace period")
			Eventually(func(g Gomega) {
				var podObj corev1.Pod
				err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "web-degraded"}, &podObj)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(podObj.Status.Phase).To(Equal(corev1.PodRunning))
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("waiting for grace period to expire")
			time.Sleep(gracePeriod + 2*time.Second)

			By("triggering re-reconciliation after grace period")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "grace-check"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Degraded condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Degraded"))
		})
	})

	Context("Grace Period — Down", func() {
		It("should report Down when Pod is not running after grace period expires", func() {
			gracePeriod := 5 * time.Second

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				p := newBasePod(ns, "web-down")
				p.Spec.Containers[0].Image = "does-not-exist:e2e-test"

				res, err := pod.NewBuilder(p).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("e2e-down").
					WithConditionType("E2EReady").
					WithResource(res, component.ResourceOptions{}).
					WithGracePeriod(gracePeriod).
					Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for the initial condition to be set")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				ShouldNot(BeNil())

			By("waiting for grace period to expire")
			time.Sleep(gracePeriod + 2*time.Second)

			By("triggering re-reconciliation after grace period")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "grace-check"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Down condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Down"))
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				p := newBasePod(ns, "web-error")
				return pod.NewBuilder(p).
					WithMutation(pod.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *pod.Mutator) error {
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
