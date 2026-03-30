//go:build e2e

package primitives

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/statefulset"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseStatefulSet(namespace, name string, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: name,
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

func newHeadlessService(namespace, name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{
					Port: 80,
					Name: "http",
				},
			},
		},
	}
}

var _ = Describe("StatefulSet Primitive", Label("statefulset"), func() {
	var (
		ns              string
		name            string
		createdServices []*corev1.Service
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-statefulset-")
		name = ns
		createdServices = nil
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		for _, svc := range createdServices {
			_ = k8sClient.Delete(ctx, svc) //nolint:errcheck
		}
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	// createHeadlessService creates the headless Service required by a StatefulSet
	// in the given namespace. This must be called before reconciliation so that the
	// StatefulSet can reference its serviceName.
	createHeadlessService := func(svcName string) {
		svc := newHeadlessService(ns, svcName)
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())
		createdServices = append(createdServices, svc)
	}

	Context("Creation and Health", func() {
		It("should create a StatefulSet and reach Healthy condition", func() {
			stsName := "web-server"
			createHeadlessService(stsName)

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				sts := newBaseStatefulSet(ns, stsName, 1)
				return statefulset.NewBuilder(sts).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the StatefulSet exists with correct spec")
			var sts appsv1.StatefulSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stsName, Namespace: ns}, &sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.27"))
			Expect(sts.Spec.ServiceName).To(Equal(stsName))

			By("verifying owner reference is set")
			expectOwnerReference(sts.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply feature mutations to the StatefulSet", func() {
			stsName := "web-mutated"
			createHeadlessService(stsName)

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				sts := newBaseStatefulSet(ns, stsName, 1)
				return statefulset.NewBuilder(sts).
					WithMutation(statefulset.Mutation{
						Name: "add-env",
						Mutate: func(m *statefulset.Mutator) error {
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

			By("verifying the env var is present on the StatefulSet")
			var sts appsv1.StatefulSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stsName, Namespace: ns}, &sts)).To(Succeed())

			envVars := sts.Spec.Template.Spec.Containers[0].Env
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

	Context("Updates", func() {
		It("should propagate image changes on re-reconciliation", func() {
			stsName := "web-update"
			createHeadlessService(stsName)

			var useUpdatedImage atomic.Bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				image := "nginx:1.27"
				if useUpdatedImage.Load() {
					image = "nginx:1.26"
				}
				sts := newBaseStatefulSet(ns, stsName, 1)
				sts.Spec.Template.Spec.Containers[0].Image = image
				return statefulset.NewBuilder(sts).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("switching the desired image and triggering reconciliation")
			useUpdatedImage.Store(true)
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				if a.Annotations == nil {
					a.Annotations = map[string]string{}
				}
				a.Annotations["e2e.ocf.io/trigger"] = "update-image"
			})

			By("verifying the StatefulSet image is updated")
			Eventually(func(g Gomega) string {
				var sts appsv1.StatefulSet
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stsName, Namespace: ns}, &sts)).To(Succeed())
				return sts.Spec.Template.Spec.Containers[0].Image
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("nginx:1.26"))

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))
		})
	})

	Context("Suspension", func() {
		It("should scale to zero when suspended and resume when un-suspended", func() {
			stsName := "web-suspend"
			createHeadlessService(stsName)

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				sts := newBaseStatefulSet(ns, stsName, 1)
				return statefulset.NewBuilder(sts).Build()
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

			By("verifying the StatefulSet is scaled to zero")
			var sts appsv1.StatefulSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stsName, Namespace: ns}, &sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(0)))

			By("un-suspending the ClusterTestApp")
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				a.Spec.Suspended = false
			})

			By("waiting for Healthy state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying replicas restored")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: stsName, Namespace: ns}, &sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded when partially available after grace period expires", func() {
			gracePeriod := 5 * time.Second
			stsName := "web-degraded"
			createHeadlessService(stsName)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				sts := newBaseStatefulSet(ns, stsName, 2)
				sts.Spec.Template.Spec.Affinity = &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": stsName},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
						},
					},
				}

				res, err := statefulset.NewBuilder(sts).Build()
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
		It("should report Down when no replicas are available after grace period expires", func() {
			gracePeriod := 5 * time.Second
			stsName := "web-down"
			createHeadlessService(stsName)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				sts := newBaseStatefulSet(ns, stsName, 1)
				sts.Spec.Template.Spec.Containers[0].Image = "does-not-exist:e2e-test"

				res, err := statefulset.NewBuilder(sts).Build()
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
			stsName := "web-error"
			createHeadlessService(stsName)

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				sts := newBaseStatefulSet(ns, stsName, 1)
				return statefulset.NewBuilder(sts).
					WithMutation(statefulset.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *statefulset.Mutator) error {
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
				sts := newBaseStatefulSet(ns, "guarded-sts", 1)
				return statefulset.NewBuilder(sts).
					WithGuard(func(_ appsv1.StatefulSet) (concepts.GuardStatusWithReason, error) {
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
