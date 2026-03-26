//go:build e2e

package primitives

import (
	"fmt"
	"sync/atomic"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/selectors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/job"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseJob(namespace, name string) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:    "worker",
							Image:   "busybox:1.36",
							Command: []string{"sh", "-c", "echo done"},
						},
					},
				},
			},
		},
	}
}

var _ = Describe("Job Primitive", Label("job"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-job-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation and Health", func() {
		It("should create a Job and reach Completed condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseJob(ns, "task-create")
				return job.NewBuilder(obj).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the Job exists with correct spec")
			var j batchv1.Job
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "task-create", Namespace: ns}, &j)).To(Succeed())
			Expect(j.Spec.Template.Spec.Containers[0].Image).To(Equal("busybox:1.36"))
			Expect(j.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"sh", "-c", "echo done"}))
			Expect(j.Spec.Template.Spec.RestartPolicy).To(Equal(corev1.RestartPolicyNever))

			By("verifying owner reference is set")
			Expect(j.OwnerReferences).NotTo(BeEmpty())
			foundOwnerRef := false
			for _, oref := range j.OwnerReferences {
				if oref.Kind == "ClusterTestApp" && oref.Name == name {
					foundOwnerRef = true
					break
				}
			}
			Expect(foundOwnerRef).To(BeTrue(), "expected owner reference with kind %q and name %q", "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply feature mutations to the Job", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseJob(ns, "task-mutated")
				return job.NewBuilder(obj).
					WithMutation(job.Mutation{
						Name: "add-env",
						Mutate: func(m *job.Mutator) error {
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

			By("verifying the env var is present on the Job")
			var j batchv1.Job
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "task-mutated", Namespace: ns}, &j)).To(Succeed())

			envVars := j.Spec.Template.Spec.Containers[0].Env
			Expect(envVars).To(
				ContainElement(corev1.EnvVar{Name: "E2E_TEST", Value: "true"}),
				"expected E2E_TEST env var on container",
			)
		})
	})

	Context("Updates", func() {
		It("should propagate label changes on re-reconciliation", func() {
			var useUpdatedLabels atomic.Bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseJob(ns, "task-update")
				if useUpdatedLabels.Load() {
					obj.Labels = map[string]string{"version": "v2"}
				} else {
					obj.Labels = map[string]string{"version": "v1"}
				}
				return job.NewBuilder(obj).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Completed state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial labels")
			var j batchv1.Job
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "task-update", Namespace: ns}, &j)).To(Succeed())
			Expect(j.Labels).To(HaveKeyWithValue("version", "v1"))

			By("switching the desired labels and triggering reconciliation")
			useUpdatedLabels.Store(true)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-labels"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying the Job labels are updated")
			Eventually(func(g Gomega) string {
				var updated batchv1.Job
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "task-update", Namespace: ns}, &updated)).To(Succeed())
				return updated.Labels["version"]
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("v2"))
		})
	})

	Context("Suspension", func() {
		It("should delete the Job when suspended and recreate when un-suspended", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseJob(ns, "task-suspend")
				return job.NewBuilder(obj).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for Completed state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("suspending the ClusterTestApp")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = true
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Suspended condition")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Suspended"))

			By("verifying the Job is deleted")
			Eventually(func() bool {
				var j batchv1.Job
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "task-suspend", Namespace: ns}, &j)
				return apierrors.IsNotFound(err)
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(BeTrue(), "expected Job to be deleted when suspended")

			By("un-suspending the ClusterTestApp")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = false
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Completed state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the Job is recreated")
			var recreated batchv1.Job
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "task-suspend", Namespace: ns}, &recreated)).To(Succeed())
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBaseJob(ns, "task-error")
				return job.NewBuilder(obj).
					WithMutation(job.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *job.Mutator) error {
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
