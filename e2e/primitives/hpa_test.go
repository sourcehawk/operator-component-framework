//go:build e2e

package primitives

import (
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/hpa"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

// newBaseHPA returns a minimal valid HPA targeting a Deployment.
func newBaseHPA(namespace, name, targetName string, minReplicas int32, maxReplicas int32) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       targetName,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: int32Ptr(80),
						},
					},
				},
			},
		},
	}
}

func int32Ptr(i int32) *int32 { return &i }

// alwaysOperational is a custom operational status handler that reports the HPA
// as operational immediately. On kind clusters without metrics-server the
// ScalingActive condition never becomes True, so we bypass that check for e2e.
func alwaysOperational(_ concepts.ConvergingOperation, _ *autoscalingv2.HorizontalPodAutoscaler) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusOperational,
		Reason: "e2e: always operational",
	}, nil
}

// createTargetDeployment creates a minimal Deployment in the given namespace
// that serves as the HPA scale target. The Deployment is created directly
// (not managed by the component framework) so HPA tests can focus on the HPA
// primitive itself.
func createTargetDeployment(ns, name string) {
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
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
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("10m"),
								},
							},
						},
					},
				},
			},
		},
	}
	ExpectWithOffset(1, k8sClient.Create(ctx, dep)).To(Succeed())
}

// alwaysPendingHPA is a custom operational status handler that always reports
// the HPA as pending, simulating a resource that never converges.
func alwaysPendingHPA(_ concepts.ConvergingOperation, _ *autoscalingv2.HorizontalPodAutoscaler) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusPending,
		Reason: "E2E: always pending",
	}, nil
}

// degradedGraceHPA is a custom grace status handler that always reports
// Degraded, simulating a resource that is partially functional after grace expiry.
func degradedGraceHPA(_ *autoscalingv2.HorizontalPodAutoscaler) (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDegraded,
		Reason: "E2E: always degraded on grace expiry",
	}, nil
}

var _ = Describe("HPA Primitive", Label("hpa"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-hpa-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create an HPA and reach Healthy condition", func() {
			targetName := "hpa-target"
			createTargetDeployment(ns, targetName)

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseHPA(ns, "app-hpa", targetName, 1, 5)
				return hpa.NewBuilder(obj).
					WithCustomOperationalStatus(alwaysOperational).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the HPA exists with correct spec")
			var h autoscalingv2.HorizontalPodAutoscaler
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-hpa", Namespace: ns}, &h)).To(Succeed())
			Expect(h.Spec.ScaleTargetRef.Name).To(Equal(targetName))
			Expect(h.Spec.ScaleTargetRef.Kind).To(Equal("Deployment"))
			Expect(*h.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(h.Spec.MaxReplicas).To(Equal(int32(5)))

			By("verifying owner reference is set")
			expectOwnerReference(h.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply spec mutations to the HPA", func() {
			targetName := "hpa-target-mutated"
			createTargetDeployment(ns, targetName)

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseHPA(ns, "app-hpa-mutated", targetName, 1, 5)
				return hpa.NewBuilder(obj).
					WithCustomOperationalStatus(alwaysOperational).
					WithMutation(hpa.Mutation{
						Name: "set-max-replicas",
						Mutate: func(m *hpa.Mutator) error {
							m.EditHPASpec(func(e *editors.HPASpecEditor) error {
								e.SetMaxReplicas(10)
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

			By("verifying the mutation was applied")
			var h autoscalingv2.HorizontalPodAutoscaler
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-hpa-mutated", Namespace: ns}, &h)).To(Succeed())
			Expect(h.Spec.MaxReplicas).To(Equal(int32(10)))
		})
	})

	Context("Updates", func() {
		It("should propagate maxReplicas changes on re-reconciliation", func() {
			targetName := "hpa-target-update"
			createTargetDeployment(ns, targetName)

			var useUpdatedSpec bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				maxReplicas := int32(5)
				if useUpdatedSpec {
					maxReplicas = 8
				}
				obj := newBaseHPA(ns, "app-hpa-update", targetName, 1, maxReplicas)
				return hpa.NewBuilder(obj).
					WithCustomOperationalStatus(alwaysOperational).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial maxReplicas")
			var h autoscalingv2.HorizontalPodAutoscaler
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-hpa-update", Namespace: ns}, &h)).To(Succeed())
			Expect(h.Spec.MaxReplicas).To(Equal(int32(5)))

			By("switching desired spec and triggering reconciliation")
			useUpdatedSpec = true
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				if a.Annotations == nil {
					a.Annotations = map[string]string{}
				}
				a.Annotations["e2e.ocf.io/trigger"] = "update-max-replicas"
			})

			By("verifying updated maxReplicas")
			Eventually(func(g Gomega) int32 {
				var updated autoscalingv2.HorizontalPodAutoscaler
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-hpa-update", Namespace: ns}, &updated)).To(Succeed())
				return updated.Spec.MaxReplicas
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal(int32(8)))
		})
	})

	Context("Suspension", func() {
		It("should delete the HPA when suspended and recreate on resume", func() {
			targetName := "hpa-target-suspend"
			createTargetDeployment(ns, targetName)

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseHPA(ns, "app-hpa-suspend", targetName, 1, 5)
				return hpa.NewBuilder(obj).
					WithCustomOperationalStatus(alwaysOperational).
					Build()
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

			By("verifying the HPA is deleted")
			Eventually(func(g Gomega) {
				var h autoscalingv2.HorizontalPodAutoscaler
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "app-hpa-suspend", Namespace: ns}, &h)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound, got: %v", err)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("un-suspending the ClusterTestApp")
			framework.UpdateClusterTestApp(ctx, k8sClient, name, func(a *framework.ClusterTestApp) {
				a.Spec.Suspended = false
			})

			By("waiting for Healthy state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the HPA is recreated")
			var h autoscalingv2.HorizontalPodAutoscaler
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-hpa-suspend", Namespace: ns}, &h)).To(Succeed())
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded after grace period expires for a non-converging resource", func() {
			gracePeriod := 5 * time.Second
			targetName := "hpa-target-grace"
			createTargetDeployment(ns, targetName)

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				obj := newBaseHPA(ns, "app-hpa-grace", targetName, 1, 5)

				res, err := hpa.NewBuilder(obj).
					WithCustomOperationalStatus(alwaysPendingHPA).
					WithCustomGraceStatus(degradedGraceHPA).
					Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("e2e-grace").
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

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			targetName := "hpa-target-error"
			createTargetDeployment(ns, targetName)

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseHPA(ns, "app-hpa-error", targetName, 1, 5)
				return hpa.NewBuilder(obj).
					WithCustomOperationalStatus(alwaysOperational).
					WithMutation(hpa.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *hpa.Mutator) error {
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
				obj := newBaseHPA(ns, "guarded-hpa", "nonexistent", 1, 5)
				return hpa.NewBuilder(obj).
					WithGuard(func(_ autoscalingv2.HorizontalPodAutoscaler) (concepts.GuardStatusWithReason, error) {
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
