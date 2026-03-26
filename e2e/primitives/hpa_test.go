//go:build e2e

package primitives

import (
	"fmt"

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

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial maxReplicas")
			var h autoscalingv2.HorizontalPodAutoscaler
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-hpa-update", Namespace: ns}, &h)).To(Succeed())
			Expect(h.Spec.MaxReplicas).To(Equal(int32(5)))

			By("switching desired spec and triggering reconciliation")
			useUpdatedSpec = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-max-replicas"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

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

			By("verifying the HPA is deleted")
			Eventually(func(g Gomega) {
				var h autoscalingv2.HorizontalPodAutoscaler
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "app-hpa-suspend", Namespace: ns}, &h)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound, got: %v", err)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())

			By("un-suspending the ClusterTestApp")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = false
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Healthy state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the HPA is recreated")
			var h autoscalingv2.HorizontalPodAutoscaler
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-hpa-suspend", Namespace: ns}, &h)).To(Succeed())
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
})
