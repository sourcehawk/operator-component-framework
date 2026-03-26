//go:build e2e

package primitives

import (
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/replicaset"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseReplicaSet(namespace, name string, replicas int32) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: appsv1.ReplicaSetSpec{
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

var _ = Describe("ReplicaSet Primitive", Label("replicaset"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-replicaset-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation and Health", func() {
		It("should create a ReplicaSet and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseReplicaSet(ns, "rs-create", 1)
				return replicaset.NewBuilder(obj).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ReplicaSet exists with correct spec")
			var rs appsv1.ReplicaSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "rs-create", Namespace: ns}, &rs)).To(Succeed())
			Expect(*rs.Spec.Replicas).To(Equal(int32(1)))
			Expect(rs.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.27"))

			By("verifying owner reference is set")
			Expect(rs.OwnerReferences).NotTo(BeEmpty())
			Expect(rs.OwnerReferences[0].Kind).To(Equal("ClusterTestApp"))
			Expect(rs.OwnerReferences[0].Name).To(Equal(name))
		})
	})

	Context("Mutations", func() {
		It("should apply feature mutations to the ReplicaSet", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseReplicaSet(ns, "rs-mutated", 1)
				return replicaset.NewBuilder(obj).
					WithMutation(replicaset.Mutation{
						Name: "add-env",
						Mutate: func(m *replicaset.Mutator) error {
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

			By("verifying the env var is present on the ReplicaSet")
			var rs appsv1.ReplicaSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "rs-mutated", Namespace: ns}, &rs)).To(Succeed())

			envVars := rs.Spec.Template.Spec.Containers[0].Env
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
			var useUpdatedImage bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				image := "nginx:1.27"
				if useUpdatedImage {
					image = "nginx:1.26"
				}
				obj := newBaseReplicaSet(ns, "rs-update", 1)
				obj.Spec.Template.Spec.Containers[0].Image = image
				return replicaset.NewBuilder(obj).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("switching the desired image and triggering reconciliation")
			useUpdatedImage = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-image"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying the ReplicaSet image is updated")
			Eventually(func(g Gomega) string {
				var rs appsv1.ReplicaSet
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "rs-update", Namespace: ns}, &rs)).To(Succeed())
				return rs.Spec.Template.Spec.Containers[0].Image
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("nginx:1.26"))

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))
		})
	})

	Context("Suspension", func() {
		It("should scale to zero when suspended and resume when un-suspended", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseReplicaSet(ns, "rs-suspend", 1)
				return replicaset.NewBuilder(obj).Build()
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

			By("verifying the ReplicaSet is scaled to zero")
			var rs appsv1.ReplicaSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "rs-suspend", Namespace: ns}, &rs)).To(Succeed())
			Expect(*rs.Spec.Replicas).To(Equal(int32(0)))

			By("un-suspending the ClusterTestApp")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = false
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Healthy state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying replicas restored")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "rs-suspend", Namespace: ns}, &rs)).To(Succeed())
			Expect(*rs.Spec.Replicas).To(Equal(int32(1)))
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded when partially available after grace period expires", func() {
			gracePeriod := 5 * time.Second

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				rsName := "rs-degraded"
				obj := newBaseReplicaSet(ns, rsName, 2)
				obj.Spec.Template.Spec.Affinity = &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"app": rsName},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
						},
					},
				}

				res, err := replicaset.NewBuilder(obj).Build()
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

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				obj := newBaseReplicaSet(ns, "rs-down", 1)
				obj.Spec.Template.Spec.Containers[0].Image = "does-not-exist:e2e-test"

				res, err := replicaset.NewBuilder(obj).Build()
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
				obj := newBaseReplicaSet(ns, "rs-error", 1)
				return replicaset.NewBuilder(obj).
					WithMutation(replicaset.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *replicaset.Mutator) error {
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
