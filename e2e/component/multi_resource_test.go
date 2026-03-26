//go:build e2e

package component

import (
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

func newDeployment(namespace, name string, replicas int32) *appsv1.Deployment {
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

func newConfigMap(namespace, name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}

var _ = Describe("Multi-Resource Component", func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-comp-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Aggregate Health", func() {
		It("should aggregate health from Deployment and ConfigMap into one condition", func() {
			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				depRes, err := deployment.NewBuilder(newDeployment(ns, "web", 1)).Build()
				if err != nil {
					return nil, err
				}

				cmRes, err := configmap.NewBuilder(newConfigMap(ns, "config", map[string]string{
					"key": "value",
				})).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("multi-resource").
					WithConditionType("E2EReady").
					WithResource(depRes, component.ResourceOptions{}).
					WithResource(cmRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying both resources exist")
			var dep appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web", Namespace: ns}, &dep)).To(Succeed())

			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("key", "value"))
		})
	})

	Context("Suspension", func() {
		It("should suspend the Deployment while leaving ConfigMap intact", func() {
			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				depRes, err := deployment.NewBuilder(newDeployment(ns, "web-sus", 1)).Build()
				if err != nil {
					return nil, err
				}

				cmRes, err := configmap.NewBuilder(newConfigMap(ns, "config-sus", map[string]string{
					"key": "value",
				})).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("suspend-test").
					WithConditionType("E2EReady").
					WithResource(depRes, component.ResourceOptions{}).
					WithResource(cmRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("suspending the component")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = true
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Suspended condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Suspended"))

			By("verifying Deployment is scaled to zero")
			var dep appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-sus", Namespace: ns}, &dep)).To(Succeed())
			Expect(*dep.Spec.Replicas).To(Equal(int32(0)))

			By("verifying ConfigMap still exists and is unchanged")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "config-sus", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("key", "value"))
		})
	})

	Context("Participation Modes", func() {
		It("should ignore auxiliary resource health in component condition", func() {
			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				// Required deployment — must be healthy for component to be healthy
				depRes, err := deployment.NewBuilder(newDeployment(ns, "web-req", 1)).Build()
				if err != nil {
					return nil, err
				}

				// Auxiliary deployment with a non-existent image — will never be ready
				auxDep := newDeployment(ns, "sidecar", 1)
				auxDep.Spec.Template.Spec.Containers[0].Image = "does-not-exist:latest"
				auxRes, err := deployment.NewBuilder(auxDep).Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("auxiliary-test").
					WithConditionType("E2EReady").
					WithResource(depRes, component.ResourceOptions{}).
					WithResource(auxRes, component.ResourceOptions{
						ParticipationMode: component.ParticipationModeAuxiliary,
					}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Healthy condition (auxiliary should not block)")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying both Deployments exist")
			var dep appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "web-req", Namespace: ns}, &dep)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "sidecar", Namespace: ns}, &dep)).To(Succeed())
		})
	})
})
