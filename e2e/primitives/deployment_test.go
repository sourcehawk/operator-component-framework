//go:build e2e

package primitives

import (
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/deployment"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseDeployment(namespace, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
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

var _ = Describe("Deployment Primitive", Label("deployment"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-deploy-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation and Health", func() {
		It("should create a Deployment and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				dep := newBaseDeployment(ns, "web-server", 1)
				return deployment.NewBuilder(dep).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the Deployment exists with correct spec")
			var dep appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-server", Namespace: ns}, &dep)).To(Succeed())
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
			Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.27"))

			By("verifying owner reference is set")
			expectOwnerReference(dep.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply feature mutations to the Deployment", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				dep := newBaseDeployment(ns, "web-mutated", 1)
				return deployment.NewBuilder(dep).
					WithMutation(deployment.Mutation{
						Name: "add-env",
						Mutate: func(m *deployment.Mutator) error {
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

			By("verifying the env var is present on the Deployment")
			var dep appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-mutated", Namespace: ns}, &dep)).To(Succeed())

			envVars := dep.Spec.Template.Spec.Containers[0].Env
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
		It("should scale to zero when suspended and resume when un-suspended", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				dep := newBaseDeployment(ns, "web-suspend", 1)
				return deployment.NewBuilder(dep).Build()
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

			By("verifying the Deployment is scaled to zero")
			var dep appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-suspend", Namespace: ns}, &dep)).To(Succeed())
			Expect(*dep.Spec.Replicas).To(Equal(int32(0)))

			By("un-suspending the ClusterTestApp")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = false
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Healthy state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying replicas restored")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-suspend", Namespace: ns}, &dep)).To(Succeed())
			Expect(*dep.Spec.Replicas).To(Equal(int32(1)))
		})
	})

	Context("Updates", func() {
		It("should propagate image changes on re-reconciliation", func() {
			// Track which image to use — starts with nginx:1.27, then switches to nginx:1.26
			var useUpdatedImage bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				image := "nginx:1.27"
				if useUpdatedImage {
					image = "nginx:1.26"
				}
				dep := newBaseDeployment(ns, "web-update", 1)
				dep.Spec.Template.Spec.Containers[0].Image = image
				return deployment.NewBuilder(dep).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("switching the desired image and triggering reconciliation")
			useUpdatedImage = true
			// Trigger reconciliation by updating the ClusterTestApp's metadata
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-image"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying the Deployment image is updated")
			Eventually(func(g Gomega) string {
				var dep appsv1.Deployment
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-update", Namespace: ns}, &dep)).To(Succeed())
				return dep.Spec.Template.Spec.Containers[0].Image
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("nginx:1.26"))

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded when partially available after grace period expires", func() {
			gracePeriod := 5 * time.Second

			// Use pod anti-affinity to prevent both replicas from scheduling on the
			// same node. Kind clusters have a single node, so only 1 of the 2 pods
			// will schedule -> ReadyReplicas=1, desired=2 -> Degraded after grace.
			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				depName := "web-degraded"
				dep := newBaseDeployment(ns, depName, 2)
				dep.Spec.Template.Spec.Affinity = &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": depName},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
						},
					},
				}

				res, err := deployment.NewBuilder(dep).Build()
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

			By("waiting for Degraded condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Degraded"))
		})
	})

	Context("Grace Period — Down", func() {
		It("should report Down when no replicas are available after grace period expires", func() {
			gracePeriod := 5 * time.Second

			// Use a non-existent image so no pods ever become ready.
			// After the grace period expires, ReadyReplicas=0 -> Down.
			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				dep := newBaseDeployment(ns, "web-down", 1)
				dep.Spec.Template.Spec.Containers[0].Image = "does-not-exist:e2e-test"

				res, err := deployment.NewBuilder(dep).Build()
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
				dep := newBaseDeployment(ns, "web-error", 1)
				return deployment.NewBuilder(dep).
					WithMutation(deployment.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *deployment.Mutator) error {
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
