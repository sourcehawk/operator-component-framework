//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pvc"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

func newBasePVC(namespace, name string, storageRequest string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageRequest),
				},
			},
		},
	}
}

// immediateOperationalStatus bypasses the default Bound check so the PVC is
// considered operational as soon as it exists. On kind clusters the default
// StorageClass uses WaitForFirstConsumer, meaning PVCs stay Pending until a
// Pod mounts them. This lets us test the primitive's reconciliation logic
// without requiring a consuming workload.
func immediateOperationalStatus(
	_ concepts.ConvergingOperation, _ *corev1.PersistentVolumeClaim,
) (concepts.OperationalStatusWithReason, error) {
	return concepts.OperationalStatusWithReason{
		Status: concepts.OperationalStatusOperational,
		Reason: "e2e: immediately operational",
	}, nil
}

var _ = Describe("pvc Primitive", Label("pvc"), func() {
	var (
		ns   string
		name string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-pvc-")
		name = ns
	})

	AfterEach(func() {
		clusterReconciler.Unregister(name)
		framework.DeleteClusterTestApp(ctx, k8sClient, name)
	})

	Context("Creation", func() {
		It("should create a PVC and reach Healthy condition", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePVC(ns, "app-data", "1Gi")
				return pvc.NewBuilder(obj).
					WithCustomOperationalStatus(immediateOperationalStatus).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the PVC exists with correct spec")
			var claim corev1.PersistentVolumeClaim
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-data", Namespace: ns}, &claim)).To(Succeed())
			Expect(claim.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
			storageReq := claim.Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storageReq.String()).To(Equal("1Gi"))

			By("verifying owner reference is set")
			Expect(claim.OwnerReferences).NotTo(BeEmpty())
			Expect(claim.OwnerReferences[0].Kind).To(Equal("ClusterTestApp"))
			Expect(claim.OwnerReferences[0].Name).To(Equal(name))
		})
	})

	Context("Mutations", func() {
		It("should apply storage mutations to the PVC", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePVC(ns, "app-data-mutated", "1Gi")
				return pvc.NewBuilder(obj).
					WithCustomOperationalStatus(immediateOperationalStatus).
					WithMutation(pvc.Mutation{
						Name: "increase-storage",
						Mutate: func(m *pvc.Mutator) error {
							m.SetStorageRequest(resource.MustParse("2Gi"))
							return nil
						},
					}).
					Build()
			})

			framework.NewClusterTestApp(ctx, k8sClient, name)

			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the mutation was applied")
			var claim corev1.PersistentVolumeClaim
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-data-mutated", Namespace: ns}, &claim)).To(Succeed())
			storageReq := claim.Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(storageReq.String()).To(Equal("2Gi"))
		})
	})

	Context("Updates", func() {
		It("should propagate storage changes on re-reconciliation", func() {
			var useUpdatedStorage bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				storage := "1Gi"
				if useUpdatedStorage {
					storage = "5Gi"
				}
				obj := newBasePVC(ns, "app-data-update", storage)
				return pvc.NewBuilder(obj).
					WithCustomOperationalStatus(immediateOperationalStatus).
					Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial storage request")
			var claim corev1.PersistentVolumeClaim
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-data-update", Namespace: ns}, &claim)).To(Succeed())
			Expect(claim.Spec.Resources.Requests[corev1.ResourceStorage]).To(Equal(resource.MustParse("1Gi")))

			By("switching desired storage and triggering reconciliation")
			useUpdatedStorage = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-storage"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated storage request")
			Eventually(func(g Gomega) resource.Quantity {
				var updated corev1.PersistentVolumeClaim
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: "app-data-update", Namespace: ns}, &updated)).To(Succeed())
				return updated.Spec.Resources.Requests[corev1.ResourceStorage]
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal(resource.MustParse("5Gi")))
		})
	})

	Context("Error", func() {
		It("should report Error condition when resource mutation fails", func() {
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePVC(ns, "app-data-error", "1Gi")
				return pvc.NewBuilder(obj).
					WithMutation(pvc.Mutation{
						Name: "failing-mutation",
						Mutate: func(m *pvc.Mutator) error {
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
