//go:build e2e

package component

import (
	"sync/atomic"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
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

	Context("Guards", func() {
		It("should block a guarded resource and report Blocked, then proceed when guard clears", func() {
			// guardOpen is flipped to 1 to unblock the guard. Shared across reconcile
			// invocations via closure, so the reconciler sees the updated value.
			var guardOpen atomic.Int32

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				// First resource: always applied, no guard
				cmRes, err := configmap.NewBuilder(newConfigMap(ns, "upstream", map[string]string{
					"role-arn": "arn:aws:iam::123456789:role/my-role",
				})).Build()
				if err != nil {
					return nil, err
				}

				// Second resource: guarded on the atomic flag
				guardedCmRes, err := configmap.NewBuilder(newConfigMap(ns, "downstream", map[string]string{
					"bucket": "my-bucket",
				})).
					WithGuard(func(_ corev1.ConfigMap) (concepts.GuardStatusWithReason, error) {
						if guardOpen.Load() == 0 {
							return concepts.GuardStatusWithReason{
								Status: concepts.GuardStatusBlocked,
								Reason: "waiting for upstream resource",
							}, nil
						}
						return concepts.GuardStatusWithReason{
							Status: concepts.GuardStatusUnblocked,
						}, nil
					}).
					Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("guard-test").
					WithConditionType("E2EReady").
					WithResource(cmRes, component.ResourceOptions{}).
					WithResource(guardedCmRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Blocked condition while guard is active")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Blocked"))

			By("verifying upstream ConfigMap was created")
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "upstream", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("role-arn", "arn:aws:iam::123456789:role/my-role"))

			By("verifying downstream ConfigMap was NOT created")
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "downstream", Namespace: ns}, &cm)
			Expect(err).To(HaveOccurred())

			By("opening the guard and triggering a reconcile")
			guardOpen.Store(1)
			// Updating the owner triggers the controller to reconcile again.
			app := &framework.ClusterTestApp{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["test.ocf.io/trigger"] = "unblock-guard"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Healthy condition after guard clears")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying downstream ConfigMap was now created")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "downstream", Namespace: ns}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKeyWithValue("bucket", "my-bucket"))
		})

		It("should extract data from resource A, unblock resource B guard, and inject into B via mutation", func() {
			// This validates the full extractor -> guard -> mutation flow end-to-end:
			// Resource A's extractor populates a shared variable. Resource B's guard
			// checks that variable to unblock. Resource B's mutation reads the variable
			// and injects it into the ConfigMap's data at Mutate() time.
			var extractedARN atomic.Value

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				// First resource: extracts the ARN after apply
				cmRes, err := configmap.NewBuilder(newConfigMap(ns, "provider-role", map[string]string{
					"arn": "arn:aws:iam::123456789:role/test",
				})).
					WithDataExtractor(func(cm corev1.ConfigMap) error {
						if v, ok := cm.Data["arn"]; ok {
							extractedARN.Store(v)
						}
						return nil
					}).
					Build()
				if err != nil {
					return nil, err
				}

				// Second resource: guard checks the extracted value, mutation injects it
				bucketRes, err := configmap.NewBuilder(newConfigMap(ns, "provider-bucket", map[string]string{
					"name": "my-bucket",
				})).
					WithGuard(func(_ corev1.ConfigMap) (concepts.GuardStatusWithReason, error) {
						v := extractedARN.Load()
						if v == nil || v.(string) == "" {
							return concepts.GuardStatusWithReason{
								Status: concepts.GuardStatusBlocked,
								Reason: "waiting for provider role ARN",
							}, nil
						}
						return concepts.GuardStatusWithReason{
							Status: concepts.GuardStatusUnblocked,
						}, nil
					}).
					WithMutation(configmap.Mutation{
						Name: "inject-role-arn",
						Mutate: func(m *configmap.Mutator) error {
							v := extractedARN.Load()
							if v != nil {
								m.EditData(func(e *editors.ConfigMapDataEditor) error {
									e.Set("role-arn", v.(string))
									return nil
								})
							}
							return nil
						},
					}).
					Build()
				if err != nil {
					return nil, err
				}

				return component.NewComponentBuilder().
					WithName("extract-guard-mutate-test").
					WithConditionType("E2EReady").
					WithResource(cmRes, component.ResourceOptions{}).
					WithResource(bucketRes, component.ResourceOptions{}).
					Suspend(owner.Spec.Suspended).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Healthy condition (extraction should unblock guard)")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying both ConfigMaps exist")
			var roleCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "provider-role", Namespace: ns}, &roleCM)).To(Succeed())

			By("verifying the mutation injected the extracted ARN into the bucket ConfigMap")
			var bucketCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "provider-bucket", Namespace: ns}, &bucketCM)).To(Succeed())
			Expect(bucketCM.Data).To(HaveKeyWithValue("name", "my-bucket"))
			Expect(bucketCM.Data).To(HaveKeyWithValue("role-arn", "arn:aws:iam::123456789:role/test"))
		})
	})
})
