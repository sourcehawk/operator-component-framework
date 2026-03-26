//go:build e2e

package primitives

import (
	"fmt"

	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

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

var _ = Describe("pv Primitive", Label("pv"), func() {
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
		// Clean up PVs since they are cluster-scoped and not garbage-collected via owner refs cross-scope
		for _, suffix := range []string{"create", "mutated", "update"} {
			pvObj := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: name + "-" + suffix}}
			_ = k8sClient.Delete(ctx, pvObj) //nolint:errcheck
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

			By("verifying owner reference is set with Kind ClusterTestApp")
			Expect(pvObj.OwnerReferences).NotTo(BeEmpty())
			Expect(pvObj.OwnerReferences[0].Kind).To(Equal("ClusterTestApp"))
			Expect(pvObj.OwnerReferences[0].Name).To(Equal(name))
		})
	})

	Context("Mutations", func() {
		It("should apply reclaim policy and storage class mutations", func() {
			pvName := name + "-mutated"
			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				obj := newBasePersistentVolume(pvName, "/tmp/e2e-pv-mutated")
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
			var useUpdatedSpec bool

			clusterReconciler.RegisterResource(name, func(owner *framework.ClusterTestApp) (component.Resource, error) {
				hostPath := "/tmp/e2e-pv-original"
				if useUpdatedSpec {
					hostPath = "/tmp/e2e-pv-updated"
				}
				obj := newBasePersistentVolume(pvName, hostPath)
				if useUpdatedSpec {
					obj.Spec.MountOptions = []string{"noexec"}
				}
				return pv.NewBuilder(obj).Build()
			})

			app := framework.NewClusterTestApp(ctx, k8sClient, name)

			By("waiting for initial Healthy state")
			Eventually(framework.GetClusterCondition(ctx, k8sClient, name, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying initial spec")
			var pvObj corev1.PersistentVolume
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, &pvObj)).To(Succeed())
			Expect(pvObj.Spec.HostPath.Path).To(Equal("/tmp/e2e-pv-original"))

			By("switching desired spec and triggering reconciliation")
			useUpdatedSpec = true
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, app)).To(Succeed())
			if app.Annotations == nil {
				app.Annotations = map[string]string{}
			}
			app.Annotations["e2e.ocf.io/trigger"] = "update-spec"
			Expect(k8sClient.Update(ctx, app)).To(Succeed())

			By("verifying updated spec")
			Eventually(func(g Gomega) string {
				var updated corev1.PersistentVolume
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, &updated)).To(Succeed())
				return updated.Spec.HostPath.Path
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(Equal("/tmp/e2e-pv-updated"))

			Eventually(func(g Gomega) []string {
				var updated corev1.PersistentVolume
				g.Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, &updated)).To(Succeed())
				return updated.Spec.MountOptions
			}, framework.DefaultTimeout, framework.DefaultPolling).Should(ContainElement("noexec"))
		})
	})

	Context("Error", func() {
		It("should report Error condition when mutation fails", func() {
			pvName := name + "-create"
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
})
