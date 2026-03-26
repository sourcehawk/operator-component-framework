//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/mutation/editors"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/ingress"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBaseIngress(namespace, name string) *networkingv1.Ingress {
	pathType := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "web",
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// alwaysOperational overrides the default handler which waits for a LoadBalancer
// address — kind has no ingress controller so we skip that check.
func alwaysOperational(_ concepts.ConvergingOperation, _ *networkingv1.Ingress) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusOperational,
		Reason: "E2E: immediately operational",
	}, nil
}

var _ = Describe("Ingress Primitive", Label("ingress"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-ingress-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create an Ingress and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ing := newBaseIngress(ns, "app-ingress")
				return ingress.NewBuilder(ing).
					WithCustomOperationalStatus(alwaysOperational).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the Ingress exists with correct spec")
			var ing networkingv1.Ingress
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-ingress", Namespace: ns}, &ing)).To(Succeed())
			Expect(ing.Spec.Rules).To(HaveLen(1))
			Expect(ing.Spec.Rules[0].Host).To(Equal("example.com"))
			Expect(ing.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name).To(Equal("web"))

			By("verifying owner reference is set")
			Expect(ing.OwnerReferences).NotTo(BeEmpty())
			Expect(ing.OwnerReferences[0].Kind).To(Equal("ClusterTestApp"))
			Expect(ing.OwnerReferences[0].Name).To(Equal(name))
		})
	})

	Context("Mutations", func() {
		It("should apply spec mutations to the Ingress", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ing := newBaseIngress(ns, "app-ingress-mutated")
				return ingress.NewBuilder(ing).
					WithCustomOperationalStatus(alwaysOperational).
					WithMutation(ingress.Mutation{
						Name: "add-tls",
						Mutate: func(m *ingress.Mutator) error {
							m.EditIngressSpec(func(e *editors.IngressSpecEditor) error {
								e.EnsureTLS(networkingv1.IngressTLS{
									Hosts:      []string{"example.com"},
									SecretName: "example-tls",
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

			By("verifying TLS is configured on the Ingress")
			var ing networkingv1.Ingress
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-ingress-mutated", Namespace: ns}, &ing)).To(Succeed())
			Expect(ing.Spec.TLS).To(HaveLen(1))
			Expect(ing.Spec.TLS[0].Hosts).To(ContainElement("example.com"))
			Expect(ing.Spec.TLS[0].SecretName).To(Equal("example-tls"))
		})
	})

	Context("Updates", func() {
		It("should propagate host changes on re-reconciliation", func() {
			var useUpdatedHost bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ing := newBaseIngress(ns, "app-ingress-update")
				if useUpdatedHost {
					ing.Spec.Rules[0].Host = "updated.example.com"
				}
				return ingress.NewBuilder(ing).
					WithCustomOperationalStatus(alwaysOperational).
					Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial host")
			var ing networkingv1.Ingress
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-ingress-update", Namespace: ns}, &ing)).To(Succeed())
			Expect(ing.Spec.Rules[0].Host).To(Equal("example.com"))

			By("switching the desired host and triggering reconciliation")
			useUpdatedHost = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-host"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying the Ingress host is updated")
			Eventually(func(g Gomega) string {
				var updated networkingv1.Ingress
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-ingress-update", Namespace: ns}, &updated)).To(Succeed())
				g.Expect(updated.Spec.Rules).NotTo(BeEmpty())
				return updated.Spec.Rules[0].Host
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("updated.example.com"))
		})
	})

	Context("Suspension", func() {
		It("should reach Suspended condition and recover when un-suspended", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ing := newBaseIngress(ns, "app-ingress-suspend")
				return ingress.NewBuilder(ing).
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

			By("verifying the Ingress still exists (no-op suspension)")
			var ing networkingv1.Ingress
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-ingress-suspend", Namespace: ns}, &ing)).To(Succeed())

			By("un-suspending the ClusterTestApp")
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			app.Spec.Suspended = false
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("waiting for Healthy state again")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))
		})
	})

	Context("Error", func() {
		It("should report Error condition when mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				ing := newBaseIngress(ns, "app-ingress-error")
				return ingress.NewBuilder(ing).
					WithCustomOperationalStatus(alwaysOperational).
					WithMutation(ingress.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *ingress.Mutator) error {
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
