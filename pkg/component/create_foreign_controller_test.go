package component

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// A resource registered with BlockOnForeignController reports Blocked, naming
// the controlling owner, instead of applying over an object that another owner
// controls (sourcehawk/operator-component-framework#199).
var _ = Describe("BlockOnForeignController", func() {
	var (
		ctx       = context.Background()
		namespace string
		ownerA    *MockOperatorCRD
		ownerB    *MockOperatorCRD
	)

	const cmName = "shared-cm"
	key := func() client.ObjectKey { return client.ObjectKey{Name: cmName, Namespace: namespace} }

	sharedConfigMapComponent := func(owner *MockOperatorCRD, opts resourceOptions) *Component {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
			Data:       map[string]string{"owner": owner.Name},
		}
		res := &MockResource{}
		res.On("Object").Return(cm, nil)
		res.On("Identity").Return("ConfigMap/" + cmName)
		res.On("Mutate", mock.Anything).Return(nil)
		return &Component{
			name:               "shared",
			conditionType:      "SharedReady",
			reconcileResources: []reconcileEntry{{Resource: res, Options: opts}},
		}
	}

	liveConfigMap := func() *corev1.ConfigMap {
		GinkgoHelper()
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key(), cm)).To(Succeed())
		return cm
	}

	BeforeEach(func() {
		namespace = createNamespace(ctx, "foreign-controller-test-")
		ownerA = &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "owner-a", Namespace: namespace}}
		ownerB = &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "owner-b", Namespace: namespace}}
		Expect(k8sClient.Create(ctx, ownerA)).To(Succeed())
		Expect(k8sClient.Create(ctx, ownerB)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, ownerA)).To(Succeed())
		Expect(k8sClient.Delete(ctx, ownerB)).To(Succeed())
	})

	It("blocks the second owner and leaves the first owner's object untouched", func() {
		Expect(sharedConfigMapComponent(ownerA, resourceOptions{}).Reconcile(ctx, newTestReconcileContext(ownerA))).To(Succeed())
		before := liveConfigMap()

		comp := sharedConfigMapComponent(ownerB, resourceOptions{BlockOnForeignController: true})
		Expect(comp.Reconcile(ctx, newTestReconcileContext(ownerB))).To(Succeed())

		cond := comp.GetCondition(ownerB)
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal(string(GuardBlocked)))
		Expect(cond.Message).To(Equal("controlled by MockOperatorCRD owner-a"))

		after := liveConfigMap()
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion), "the blocked owner must not write the object")
		Expect(after.Data).To(HaveKeyWithValue("owner", ownerA.Name))
		Expect(after.OwnerReferences).To(Equal(before.OwnerReferences))
		Expect(after.ManagedFields).To(Equal(before.ManagedFields))
	})

	It("blocks an unowned registration when another owner's controller reference is on the object", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
			Data:       map[string]string{"owner": ownerA.Name},
		}
		Expect(controllerutil.SetControllerReference(ownerA, cm, scheme.Scheme)).To(Succeed())
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		Expect(sharedConfigMapComponent(ownerA, resourceOptions{Unowned: true}).Reconcile(ctx, newTestReconcileContext(ownerA))).To(Succeed())
		before := liveConfigMap()

		comp := sharedConfigMapComponent(ownerB, resourceOptions{Unowned: true, BlockOnForeignController: true})
		Expect(comp.Reconcile(ctx, newTestReconcileContext(ownerB))).To(Succeed())
		Expect(comp.GetCondition(ownerB).Reason).To(Equal(string(GuardBlocked)))
		Expect(liveConfigMap().ResourceVersion).To(Equal(before.ResourceVersion))
	})

	It("does not block an unowned registration when the object has no controller", func() {
		Expect(sharedConfigMapComponent(ownerA, resourceOptions{Unowned: true}).Reconcile(ctx, newTestReconcileContext(ownerA))).To(Succeed())

		comp := sharedConfigMapComponent(ownerB, resourceOptions{Unowned: true, BlockOnForeignController: true})
		Expect(comp.Reconcile(ctx, newTestReconcileContext(ownerB))).To(Succeed())
		Expect(comp.GetCondition(ownerB).Reason).NotTo(Equal(string(GuardBlocked)))
		Expect(liveConfigMap().Data).To(HaveKeyWithValue("owner", ownerB.Name))
	})

	It("does not block the owner that controls the object", func() {
		comp := sharedConfigMapComponent(ownerA, resourceOptions{BlockOnForeignController: true})
		rec := newTestReconcileContext(ownerA)
		Expect(comp.Reconcile(ctx, rec)).To(Succeed())
		Expect(comp.Reconcile(ctx, rec)).To(Succeed())
		Expect(comp.GetCondition(ownerA).Reason).NotTo(Equal(string(GuardBlocked)))
		Expect(liveConfigMap().OwnerReferences[0].UID).To(Equal(ownerA.UID))
	})

	It("reports a suspended component without scaling down or deleting the object another owner controls", func() {
		Expect(sharedConfigMapComponent(ownerA, resourceOptions{}).Reconcile(ctx, newTestReconcileContext(ownerA))).To(Succeed())
		before := liveConfigMap()

		desired := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace}}
		res := &MockSuspendableResource{}
		res.On("Object").Return(desired, nil)
		res.On("Identity").Return("ConfigMap/" + cmName)
		res.On("DeleteOnSuspend").Return(true)
		// No Suspend, Mutate or SuspensionStatus expectation: reaching any of them
		// means the component was about to apply or delete the other owner's object.
		comp := &Component{
			name:          "shared",
			conditionType: "SharedReady",
			suspended:     true,
			reconcileResources: []reconcileEntry{{
				Resource: res, Options: resourceOptions{Unowned: true, BlockOnForeignController: true},
			}},
		}

		Expect(comp.Reconcile(ctx, newTestReconcileContext(ownerB))).To(Succeed())
		cond := comp.GetCondition(ownerB)
		Expect(cond.Reason).To(Equal(string(Suspended)))
		Expect(cond.Message).To(Equal("All resources are suspended."))

		after := liveConfigMap()
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion))
		Expect(after.OwnerReferences[0].UID).To(Equal(ownerA.UID))
	})

	It("unblocks once the controlling owner's reference is gone", func() {
		Expect(sharedConfigMapComponent(ownerA, resourceOptions{}).Reconcile(ctx, newTestReconcileContext(ownerA))).To(Succeed())

		comp := sharedConfigMapComponent(ownerB, resourceOptions{BlockOnForeignController: true})
		rec := newTestReconcileContext(ownerB)
		Expect(comp.Reconcile(ctx, rec)).To(Succeed())
		Expect(comp.GetCondition(ownerB).Reason).To(Equal(string(GuardBlocked)))

		released := liveConfigMap()
		released.OwnerReferences = nil
		Expect(k8sClient.Update(ctx, released)).To(Succeed())

		Expect(comp.Reconcile(ctx, rec)).To(Succeed())
		Expect(comp.GetCondition(ownerB).Reason).NotTo(Equal(string(GuardBlocked)))
		after := liveConfigMap()
		Expect(after.Data).To(HaveKeyWithValue("owner", ownerB.Name))
		Expect(after.OwnerReferences).To(HaveLen(1))
		Expect(after.OwnerReferences[0].UID).To(Equal(ownerB.UID))
	})
})
