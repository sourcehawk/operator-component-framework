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
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBasePersistentVolume(name, hostPath string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("1Gi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath,
				},
			},
		},
	}
}

// alwaysPendingPV always reports the PV as pending so it never converges,
// allowing the grace period to expire.
func alwaysPendingPV(
	_ concepts.ConvergingOperation, _ *corev1.PersistentVolume,
) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusPending,
		Reason: "e2e: always pending",
	}, nil
}

// degradedGracePV reports the PV as degraded when the grace period expires.
func degradedGracePV(_ *corev1.PersistentVolume) (concepts.GraceStatusWithReason, error) {
	return concepts.GraceStatusWithReason{
		Status: concepts.GraceStatusDegraded,
		Reason: "e2e: degraded after grace",
	}, nil
}

var _ = Describe("PersistentVolume Primitive", Label("pv"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-pv-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
		// Explicitly clean up cluster-scoped PVs to avoid leaking test resources and relying on GC timing
		for _, suffix := range []string{"create", "mutated", "update", "grace", "error", "guard"} {
			pvObj := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: name + "-" + suffix}}
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, pvObj))).To(Succeed())
		}
	})

	Context("Creation", func() {
		It("should create a PersistentVolume and reach Healthy condition", func() {
			pvName := name + "-create"
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePersistentVolume(pvName, "/tmp/e2e-pv-create")
				return pv.NewBuilder(obj).Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the PersistentVolume exists with correct spec")
			var pvObj corev1.PersistentVolume
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, &pvObj)).To(Succeed())
			Expect(pvObj.Spec.Capacity).To(HaveKeyWithValue(corev1.ResourceStorage, resource.MustParse("1Gi")))
			Expect(pvObj.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
			Expect(pvObj.Spec.HostPath.Path).To(Equal("/tmp/e2e-pv-create"))

			By("verifying owner reference is set")
			expectOwnerReference(pvObj.ObjectMeta, "ClusterTestApp", name)
		})
	})

	Context("Mutations", func() {
		It("should apply reclaim policy and storage class mutations", func() {
			pvName := name + "-mutated"
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePersistentVolume(pvName, "/tmp/e2e-pv-mutated")
				obj.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimDelete
				return pv.NewBuilder(obj).
					WithMutation(pv.Mutation{
						Name: "set-reclaim-policy",
						Mutate: func(m *pv.Mutator) error {
							m.SetReclaimPolicy(corev1.PersistentVolumeReclaimRetain)
							return nil
						},
					}).
					WithMutation(pv.Mutation{
						Name: "set-storage-class",
						Mutate: func(m *pv.Mutator) error {
							m.SetStorageClassName("manual")
							return nil
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying mutations are applied")
			var pvObj corev1.PersistentVolume
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, &pvObj)).To(Succeed())
			Expect(pvObj.Spec.PersistentVolumeReclaimPolicy).To(Equal(corev1.PersistentVolumeReclaimRetain))
			Expect(pvObj.Spec.StorageClassName).To(Equal("manual"))
		})
	})

	Context("Updates", func() {
		It("should propagate spec changes on re-reconciliation", func() {
			pvName := name + "-update"
			var useUpdatedSpec atomic.Bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePersistentVolume(pvName, "/tmp/e2e-pv-update")
				obj.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimDelete
				builder := pv.NewBuilder(obj)
				if useUpdatedSpec.Load() {
					builder = builder.WithMutation(pv.Mutation{
						Name: "set-reclaim-retain",
						Mutate: func(m *pv.Mutator) error {
							m.SetReclaimPolicy(corev1.PersistentVolumeReclaimRetain)
							return nil
						},
					}).WithMutation(pv.Mutation{
						Name: "add-label",
						Mutate: func(m *pv.Mutator) error {
							m.EditObjectMetadata(func(e *editors.ObjectMetaEditor) error {
								e.EnsureLabel("e2e.ocf.io/updated", "true")
								return nil
							})
							return nil
						},
					})
				}
				return builder.Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial spec")
			var pvObj corev1.PersistentVolume
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, &pvObj)).To(Succeed())
			Expect(pvObj.Spec.HostPath.Path).To(Equal("/tmp/e2e-pv-update"))
			Expect(pvObj.Spec.PersistentVolumeReclaimPolicy).To(Equal(corev1.PersistentVolumeReclaimDelete))

			By("switching desired spec and triggering reconciliation")
			useUpdatedSpec.Store(true)
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-spec"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated reclaim policy")
			Eventually(func(g Gomega) corev1.PersistentVolumeReclaimPolicy {
				var updated corev1.PersistentVolume
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, &updated)).To(Succeed())
				return updated.Spec.PersistentVolumeReclaimPolicy
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal(corev1.PersistentVolumeReclaimRetain))

			By("verifying updated label")
			Eventually(func(g Gomega) map[string]string {
				var updated corev1.PersistentVolume
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, &updated)).To(Succeed())
				return updated.Labels
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(HaveKeyWithValue("e2e.ocf.io/updated", "true"))
		})
	})

	Context("Grace Period — Degraded", func() {
		It("should report Degraded after grace period expires for a non-converging resource", func() {
			gracePeriod := 5 * time.Second

			clusterReconciler.RegisterComponent(name, func(owner *framework.ClusterTestApp) (*component.Component, error) {
				obj := newBasePersistentVolume(name+"-grace", "/tmp/e2e-pv-grace")

				res, err := pv.NewBuilder(obj).
					WithCustomOperationalStatus(alwaysPendingPV).
					WithCustomGraceStatus(degradedGracePV).
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
		It("should report Error condition when mutation fails", func() {
			pvName := name + "-error"
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePersistentVolume(pvName, "/tmp/e2e-pv-error")
				return pv.NewBuilder(obj).
					WithMutation(pv.Mutation{
						Name: "intentional-failure",
						Mutate: func(m *pv.Mutator) error {
							return fmt.Errorf("intentional e2e mutation failure")
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionFalse, "Error"))
		})
	})

	Context("Guards", func() {
		It("should report Blocked condition when guard blocks", func() {
			pvName := name + "-guard"
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePersistentVolume(pvName, "/tmp/e2e-pv-guard")
				return pv.NewBuilder(obj).
					WithGuard(func(_ corev1.PersistentVolume) (concepts.GuardStatusWithReason, error) {
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
