package component

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Two owners of one kind whose components render the same object must not
// share a field manager. With a shared manager, each owner's forced apply
// silently relinquishes the other's fields and both report converged
// (sourcehawk/operator-component-framework#197). With a per-owner manager the
// API server sees two managers and refuses the second owner's controller
// reference, so the second owner fails instead of stealing the object.
var _ = Describe("Apply field manager", func() {
	var (
		ctx       = context.Background()
		namespace string
		ownerA    *MockOperatorCRD
		ownerB    *MockOperatorCRD
	)

	const componentName = "shared"

	// sharedConfigMapComponent renders the ConfigMap "shared-cm" with data
	// tagged by the owner that rendered it, exactly as two custom resources
	// naming one config object would.
	sharedConfigMapComponent := func(owner *MockOperatorCRD) *Component {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "shared-cm", Namespace: namespace},
			Data:       map[string]string{"owner": owner.Name},
		}
		res := &MockResource{}
		res.On("Object").Return(cm, nil)
		res.On("Identity").Return("ConfigMap/shared-cm")
		res.On("Mutate", mock.Anything).Return(nil)
		return &Component{
			name:               componentName,
			conditionType:      "SharedReady",
			reconcileResources: []reconcileEntry{{Resource: res}},
		}
	}

	BeforeEach(func() {
		namespace = createNamespace(ctx, "field-manager-test-")
		ownerA = &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "owner-a", Namespace: namespace}}
		ownerB = &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "owner-b", Namespace: namespace}}
		Expect(k8sClient.Create(ctx, ownerA)).To(Succeed())
		Expect(k8sClient.Create(ctx, ownerB)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, ownerA)).To(Succeed())
		Expect(k8sClient.Delete(ctx, ownerB)).To(Succeed())
	})

	applyManagers := func(cm *corev1.ConfigMap) []string {
		var managers []string
		for _, entry := range cm.ManagedFields {
			if entry.Operation == metav1.ManagedFieldsOperationApply {
				managers = append(managers, entry.Manager)
			}
		}
		return managers
	}

	It("names the owner in the field manager so a second owner cannot take the object", func() {
		Expect(sharedConfigMapComponent(ownerA).Reconcile(ctx, newTestReconcileContext(ownerA))).To(Succeed())

		err := sharedConfigMapComponent(ownerB).Reconcile(ctx, newTestReconcileContext(ownerB))
		Expect(err).To(HaveOccurred(), "the second owner's apply must be refused, not silently take over")

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "shared-cm", Namespace: namespace}, cm)).To(Succeed())

		Expect(cm.Data).To(HaveKeyWithValue("owner", ownerA.Name))
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(cm.OwnerReferences[0].UID).To(Equal(ownerA.UID))
		Expect(applyManagers(cm)).To(ConsistOf("MockOperatorCRD/" + componentName + "/" + string(ownerA.UID)))
	})

	It("applies with a hashed manager when the readable name exceeds the API server limit", func() {
		comp := sharedConfigMapComponent(ownerA)
		comp.name = strings.Repeat("c", 128)
		Expect(comp.Reconcile(ctx, newTestReconcileContext(ownerA))).To(Succeed())

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "shared-cm", Namespace: namespace}, cm)).To(Succeed())
		Expect(cm.Data).To(HaveKeyWithValue("owner", ownerA.Name))
		Expect(applyManagers(cm)).To(ConsistOf(string(applyFieldOwner(ownerA, comp.name))))
		Expect(applyManagers(cm)[0]).To(HaveLen(64))
	})

	It("keeps one manager per owner across repeated reconciles", func() {
		comp := sharedConfigMapComponent(ownerA)
		rec := newTestReconcileContext(ownerA)
		Expect(comp.Reconcile(ctx, rec)).To(Succeed())
		Expect(comp.Reconcile(ctx, rec)).To(Succeed())

		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "shared-cm", Namespace: namespace}, cm)).To(Succeed())
		Expect(applyManagers(cm)).To(HaveLen(1))
	})
})
