//go:build e2e

package primitives

import (
	"github.com/sourcehawk/operator-component-framework/e2e/framework"
	"github.com/sourcehawk/operator-component-framework/pkg/component"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrole"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/clusterrolebinding"
	"github.com/sourcehawk/operator-component-framework/pkg/primitives/pv"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
)

var _ = Describe("Cross-Scope Resource Management", func() {
	var (
		ns      string
		key     types.NamespacedName
		appName string
	)

	BeforeEach(func() {
		ns = framework.CreateTestNamespace(ctx, k8sClient, "e2e-cross-scope-")
		appName = "cross-scope-" + ns
		key = types.NamespacedName{Namespace: ns, Name: appName}
	})

	AfterEach(func() {
		reconciler.Unregister(key)
	})

	Context("ClusterRole", func() {
		var crName string

		BeforeEach(func() {
			crName = "e2e-cr-" + ns
		})

		AfterEach(func() {
			cr := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: crName}}
			_ = k8sClient.Delete(ctx, cr) //nolint:errcheck
		})

		It("should create a ClusterRole without owner references", func() {
			reconciler.RegisterResource(key, func(owner *framework.TestApp) (component.Resource, error) {
				cr := &rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: crName,
						Labels: map[string]string{
							"e2e.ocf.io/owner-namespace": owner.Namespace,
							"e2e.ocf.io/owner-name":      owner.Name,
						},
					},
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"configmaps"},
							Verbs:     []string{"get", "list", "watch"},
						},
					},
				}
				return clusterrole.NewBuilder(cr).Build()
			})

			framework.NewTestApp(ctx, k8sClient, ns, appName)

			Eventually(framework.GetCondition(ctx, k8sClient, key, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ClusterRole exists with correct rules")
			var cr rbacv1.ClusterRole
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crName}, &cr)).To(Succeed())
			Expect(cr.Rules).To(HaveLen(1))
			Expect(cr.Rules[0].Resources).To(ContainElement("configmaps"))

			By("verifying no owner reference is set")
			Expect(cr.OwnerReferences).To(BeEmpty())
		})
	})

	Context("ClusterRoleBinding", func() {
		var crbName string

		BeforeEach(func() {
			crbName = "e2e-crb-" + ns
		})

		AfterEach(func() {
			crb := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: crbName}}
			_ = k8sClient.Delete(ctx, crb) //nolint:errcheck
		})

		It("should create a ClusterRoleBinding without owner references", func() {
			reconciler.RegisterResource(key, func(owner *framework.TestApp) (component.Resource, error) {
				crb := &rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: crbName,
						Labels: map[string]string{
							"e2e.ocf.io/owner-namespace": owner.Namespace,
							"e2e.ocf.io/owner-name":      owner.Name,
						},
					},
					RoleRef: rbacv1.RoleRef{
						APIGroup: "rbac.authorization.k8s.io",
						Kind:     "ClusterRole",
						Name:     "view",
					},
					Subjects: []rbacv1.Subject{
						{
							Kind:      "ServiceAccount",
							Name:      "default",
							Namespace: ns,
						},
					},
				}
				return clusterrolebinding.NewBuilder(crb).Build()
			})

			framework.NewTestApp(ctx, k8sClient, ns, appName)

			Eventually(framework.GetCondition(ctx, k8sClient, key, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the ClusterRoleBinding exists with correct role ref")
			var crb rbacv1.ClusterRoleBinding
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: crbName}, &crb)).To(Succeed())
			Expect(crb.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(crb.RoleRef.Name).To(Equal("view"))
			Expect(crb.Subjects).To(HaveLen(1))

			By("verifying no owner reference is set")
			Expect(crb.OwnerReferences).To(BeEmpty())
		})
	})

	Context("PersistentVolume", func() {
		var pvName string

		BeforeEach(func() {
			pvName = "e2e-pv-" + ns
		})

		AfterEach(func() {
			pvObj := &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: pvName}}
			_ = k8sClient.Delete(ctx, pvObj) //nolint:errcheck
		})

		It("should create a PersistentVolume without owner references", func() {
			reconciler.RegisterResource(key, func(owner *framework.TestApp) (component.Resource, error) {
				pvObj := &corev1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: pvName,
						Labels: map[string]string{
							"e2e.ocf.io/owner-namespace": owner.Namespace,
							"e2e.ocf.io/owner-name":      owner.Name,
						},
					},
					Spec: corev1.PersistentVolumeSpec{
						Capacity: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						PersistentVolumeSource: corev1.PersistentVolumeSource{
							HostPath: &corev1.HostPathVolumeSource{
								Path: "/tmp/e2e-pv-test",
							},
						},
					},
				}
				return pv.NewBuilder(pvObj).Build()
			})

			framework.NewTestApp(ctx, k8sClient, ns, appName)

			Eventually(framework.GetCondition(ctx, k8sClient, key, "E2EReady"), framework.DefaultTimeout, framework.DefaultPolling).
				Should(framework.HaveConditionStatus(metav1.ConditionTrue, "Healthy"))

			By("verifying the PersistentVolume exists with correct spec")
			var pvObj corev1.PersistentVolume
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pvName}, &pvObj)).To(Succeed())
			Expect(pvObj.Spec.Capacity).To(HaveKeyWithValue(corev1.ResourceStorage, resource.MustParse("1Gi")))
			Expect(pvObj.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))

			By("verifying no owner reference is set")
			Expect(pvObj.OwnerReferences).To(BeEmpty())
		})
	})
})
