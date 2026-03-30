//go:build e2e

package primitives

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/cronjob"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseCronJob(namespace, name string) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "* * * * *",
			Suspend:  ptr.To(true),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{
								{
									Name:    "worker",
									Image:   "busybox:1.36",
									Command: []string{"echo", "hello"},
								},
							},
						},
					},
				},
			},
		},
	}
}

// immediatelyOperational overrides the default operational status handler to
// report the CronJob as operational immediately, avoiding the need to wait
// for LastScheduleTime to be set.
func immediatelyOperational(_ concepts.ConvergingOperation, _ *batchv1.CronJob) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusOperational,
		Reason: "e2e: immediately operational",
	}, nil
}

// alwaysPendingCronJob always reports the CronJob as pending so it never
// converges, allowing the grace period to expire.
func alwaysPendingCronJob(
	_ concepts.ConvergingOperation, _ *batchv1.CronJob,
) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusPending,
		Reason: "e2e: always pending",
	}, nil
}

// degradedGraceCronJob reports the CronJob as degraded when the grace period expires.
func degradedGraceCronJob(_ *batchv1.CronJob) (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDegraded,
		Reason: "e2e: degraded after grace",
	}, nil
}

var _ = Describe("CronJob Primitive", Label("cronjob"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-cronjob-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a CronJob and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cj := newBaseCronJob(ns, "cron-create")
				return cronjob.NewBuilder(cj).
					WithCustomOperationalStatus(immediatelyOperational).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the CronJob exists with correct spec")
			var cj batchv1.CronJob
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cron-create", Namespace: ns}, &cj)).To(Succeed())
			Expect(cj.Spec.Schedule).To(Equal("* * * * *"))
			Expect(cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image).To(Equal("busybox:1.36"))

			By("verifying owner reference is set")
			expectOwnerReference(cj.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply mutations to the CronJob", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cj := newBaseCronJob(ns, "cron-mutated")
				return cronjob.NewBuilder(cj).
					WithCustomOperationalStatus(immediatelyOperational).
					WithMutation(cronjob.Mutation{
						Name: "add-env",
						Mutate: func(m *cronjob.Mutator) error {
							m.EnsureContainerEnvVar(corev1.EnvVar{Name: "E2E_TEST", Value: "true"})
							return nil
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the env var is present on the CronJob")
			var cj batchv1.CronJob
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cron-mutated", Namespace: ns}, &cj)).To(Succeed())

			envVars := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Env
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
		It("should propagate schedule changes on re-reconciliation", func() {
			var useUpdatedSchedule atomic.Bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cj := newBaseCronJob(ns, "cron-update")
				if useUpdatedSchedule.Load() {
					cj.Spec.Schedule = "*/5 * * * *"
				}
				return cronjob.NewBuilder(cj).
					WithCustomOperationalStatus(immediatelyOperational).
					Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial schedule")
			var cj batchv1.CronJob
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cron-update", Namespace: ns}, &cj)).To(Succeed())
			Expect(cj.Spec.Schedule).To(Equal("* * * * *"))

			By("switching the desired schedule and triggering reconciliation")
			useUpdatedSchedule.Store(true)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-schedule"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying the CronJob schedule is updated")
			Eventually(func(g Gomega) string {
				var updated batchv1.CronJob
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cron-update", Namespace: ns}, &updated)).To(Succeed())
				return updated.Spec.Schedule
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("*/5 * * * *"))
		})
	})

	Context("Suspension", func() {
		It("should suspend the CronJob when owner is suspended and resume when un-suspended", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cj := newBaseCronJob(ns, "cron-suspend")
				cj.Spec.Suspend = ptr.To(false) // override base suspend so we can test suspension transitions
				return cronjob.NewBuilder(cj).
					WithCustomOperationalStatus(immediatelyOperational).
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

			By("verifying the CronJob has spec.suspend set to true")
			var cj batchv1.CronJob
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cron-suspend", Namespace: ns}, &cj)).To(Succeed())
			Expect(cj.Spec.Suspend).NotTo(BeNil())
			Expect(*cj.Spec.Suspend).To(BeTrue())

			By("un-suspending the ClusterTestApp")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = false
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Healthy state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the CronJob is no longer suspended")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "cron-suspend", Namespace: ns}, &cj)).To(Succeed())
			if cj.Spec.Suspend != nil {
				Expect(*cj.Spec.Suspend).To(BeFalse())
			}
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded after grace period expires for a non-converging resource", func() {
			gracePeriod := 5 * time.Second

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				obj := newBaseCronJob(ns, "cron-grace")

				res, err := cronjob.NewBuilder(obj).
					WithCustomOperationalStatus(alwaysPendingCronJob).
					WithCustomGraceStatus(degradedGraceCronJob).
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

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				cj := newBaseCronJob(ns, "cron-error")
				return cronjob.NewBuilder(cj).
					WithCustomOperationalStatus(immediatelyOperational).
					WithMutation(cronjob.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *cronjob.Mutator) error {
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
				cj := newBaseCronJob(ns, "guarded-cron")
				return cronjob.NewBuilder(cj).
					WithGuard(func(_ batchv1.CronJob) (concepts.GuardStatusWithReason, error) {
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
