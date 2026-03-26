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
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/service"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseService(namespace, name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// alwaysPendingService is a custom operational status handler that always
// reports the Service as pending, simulating a resource that never converges.
func alwaysPendingService(_ concepts.ConvergingOperation, _ *corev1.Service) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusPending,
		Reason: "E2E: always pending",
	}, nil
}

// degradedGraceService is a custom grace status handler that always reports
// Degraded, simulating a resource that is partially functional after grace expiry.
func degradedGraceService(_ *corev1.Service) (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDegraded,
		Reason: "E2E: always degraded on grace expiry",
	}, nil
}

var _ = Describe("Service Primitive", Label("service"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-service-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a Service and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				svc := newBaseService(ns, "app-svc")
				return service.NewBuilder(svc).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the Service exists with correct spec")
			var svc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-svc", Namespace: ns}, &svc)).To(Succeed())
			Expect(svc.Spec.Selector).To(HaveKeyWithValue("app", "app-svc"))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports).To(ContainElement(HaveField("Port", Equal(int32(80)))))

			By("verifying owner reference is set")
			expectOwnerReference(svc.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply spec mutations to the Service", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				svc := newBaseService(ns, "app-svc-mutated")
				return service.NewBuilder(svc).
					WithMutation(service.Mutation{
						Name: "add-extra-port",
						Mutate: func(m *service.Mutator) error {
							m.EditServiceSpec(func(e *editors.ServiceSpecEditor) error {
								e.EnsurePort(corev1.ServicePort{
									Name:       "metrics",
									Port:       9090,
									TargetPort: intstr.FromInt32(9090),
									Protocol:   corev1.ProtocolTCP,
								})
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

			By("verifying both base and mutated ports are present")
			var svc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-svc-mutated", Namespace: ns}, &svc)).To(Succeed())
			Expect(svc.Spec.Ports).To(HaveLen(2))
			portNames := []string{svc.Spec.Ports[0].Name, svc.Spec.Ports[1].Name}
			Expect(portNames).To(ContainElements("http", "metrics"))
		})
	})

	Context("Updates", func() {
		It("should propagate spec changes on re-reconciliation", func() {
			var useUpdatedSpec atomic.Bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				svc := newBaseService(ns, "app-svc-update")
				if useUpdatedSpec.Load() {
					svc.Spec.Ports = []corev1.ServicePort{
						{
							Name:       "https",
							Port:       443,
							TargetPort: intstr.FromInt32(8443),
							Protocol:   corev1.ProtocolTCP,
						},
					}
				}
				return service.NewBuilder(svc).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial port")
			var svc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-svc-update", Namespace: ns}, &svc)).To(Succeed())
			Expect(svc.Spec.Ports).To(ContainElement(HaveField("Port", Equal(int32(80)))))

			By("switching desired spec and triggering reconciliation")
			useUpdatedSpec.Store(true)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-spec"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated port")
			Eventually(func(g Gomega) {
				var updated corev1.Service
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-svc-update", Namespace: ns}, &updated)).To(Succeed())
				g.Expect(updated.Spec.Ports).To(HaveLen(1))
				g.Expect(updated.Spec.Ports).To(ContainElement(HaveField("Port", Equal(int32(443)))))
				g.Expect(updated.Spec.Ports).To(Not(ContainElement(HaveField("Port", Equal(int32(80))))))
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Succeed())
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded after grace period expires for a non-converging resource", func() {
			gracePeriod := 5 * time.Second

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				svc := newBaseService(ns, "app-svc-grace")

				res, err := service.NewBuilder(svc).
					WithCustomOperationalStatus(alwaysPendingService).
					WithCustomGraceStatus(degradedGraceService).
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
				svc := newBaseService(ns, "app-svc-error")
				return service.NewBuilder(svc).
					WithMutation(service.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *service.Mutator) error {
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
