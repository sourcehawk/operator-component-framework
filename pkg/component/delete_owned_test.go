package component

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// A resource registered with DeleteOnlyIfOwned is deleted by a deletion
// trigger only when the live object carries the owner's reference, so an
// object a user created under the same name survives
// (sourcehawk/operator-component-framework#233).
var _ = Describe("DeleteOnlyIfOwned", func() {
	var (
		ctx       = context.Background()
		namespace string
		owner     *MockOperatorCRD
	)

	const cmName = "gated-cm"
	key := func() client.ObjectKey { return client.ObjectKey{Name: cmName, Namespace: namespace} }

	// trigger builds a component whose only resource is asked to be deleted by
	// one of the deletion triggers, with the extra resource options appended.
	type trigger func(res Resource, opts ...ResourceOption) (*Component, error)

	gatedBy := func(res Resource, opts ...ResourceOption) (*Component, error) {
		return NewComponentBuilder().
			WithName("gated").
			WithConditionType("GatedReady").
			WithResource(res, append([]ResourceOption{GatedBy(&testGate{enabled: false})}, opts...)...).
			Build()
	}
	deleteWhen := func(res Resource, opts ...ResourceOption) (*Component, error) {
		return NewComponentBuilder().
			WithName("gated").
			WithConditionType("GatedReady").
			WithResource(res, append([]ResourceOption{DeleteWhen(true)}, opts...)...).
			Build()
	}
	disabledComponentGate := func(res Resource, opts ...ResourceOption) (*Component, error) {
		return NewComponentBuilder().
			WithName("gated").
			WithConditionType("GatedReady").
			WithFeatureGate(&testGate{enabled: false}).
			WithResource(res, opts...).
			Build()
	}

	desiredResource := func() *MockResource {
		res := &MockResource{}
		res.On("Object").Return(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace}}, nil)
		res.On("Identity").Return("ConfigMap/" + cmName)
		return res
	}

	createConfigMap := func(owned bool) *corev1.ConfigMap {
		GinkgoHelper()
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace},
			Data:       map[string]string{"created-by": "user"},
		}
		if owned {
			Expect(controllerutil.SetControllerReference(owner, cm, scheme.Scheme)).To(Succeed())
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		return cm
	}

	BeforeEach(func() {
		namespace = createNamespace(ctx, "delete-owned-test-")
		owner = &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: namespace}}
		Expect(k8sClient.Create(ctx, owner)).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, owner)).To(Succeed())
	})

	DescribeTable("deletes an object that carries the owner's reference",
		func(build trigger) {
			createConfigMap(true)
			comp, err := build(desiredResource(), DeleteOnlyIfOwned())
			Expect(err).NotTo(HaveOccurred())

			rec := newTestReconcileContext(owner)
			Expect(comp.Reconcile(ctx, rec)).To(Succeed())

			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, key(), &corev1.ConfigMap{}))).To(BeTrue())
			Expect(rec.EventRecorder.(*spyRecorder).recordedWithReason("ResourceDeleted")).To(HaveLen(1))
		},
		Entry("GatedBy with a disabled gate", trigger(gatedBy)),
		Entry("DeleteWhen(true)", trigger(deleteWhen)),
		Entry("a disabled component feature gate", trigger(disabledComponentGate)),
	)

	DescribeTable("keeps an object without the owner's reference unchanged",
		func(build trigger) {
			createConfigMap(false)
			before := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, key(), before)).To(Succeed())
			comp, err := build(desiredResource(), DeleteOnlyIfOwned())
			Expect(err).NotTo(HaveOccurred())

			rec := newTestReconcileContext(owner)
			Expect(comp.Reconcile(ctx, rec)).To(Succeed())

			after := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, key(), after)).To(Succeed())
			Expect(after.ResourceVersion).To(Equal(before.ResourceVersion), "a kept object must not be written")
			Expect(after.Data).To(HaveKeyWithValue("created-by", "user"))
			Expect(rec.EventRecorder.(*spyRecorder).recordedWithReason("ResourceDeleted")).To(BeEmpty())
		},
		Entry("GatedBy with a disabled gate", trigger(gatedBy)),
		Entry("DeleteWhen(true)", trigger(deleteWhen)),
		Entry("a disabled component feature gate", trigger(disabledComponentGate)),
	)

	DescribeTable("without the option, still deletes an object without the owner's reference",
		func(build trigger) {
			createConfigMap(false)
			comp, err := build(desiredResource())
			Expect(err).NotTo(HaveOccurred())

			Expect(comp.Reconcile(ctx, newTestReconcileContext(owner))).To(Succeed())

			Expect(apierrors.IsNotFound(k8sClient.Get(ctx, key(), &corev1.ConfigMap{}))).To(BeTrue())
		},
		Entry("GatedBy with a disabled gate", trigger(gatedBy)),
		Entry("DeleteWhen(true)", trigger(deleteWhen)),
		Entry("a disabled component feature gate", trigger(disabledComponentGate)),
	)

	suspendedComponent := func(res Resource) *Component {
		return &Component{
			name:          "gated",
			conditionType: "GatedReady",
			suspended:     true,
			reconcileResources: []reconcileEntry{{
				Resource: res, Options: resourceOptions{DeleteOnlyIfOwned: true},
			}},
		}
	}

	It("deletes an owned object on suspension with DeleteOnSuspend", func() {
		createConfigMap(true)
		res := &MockSuspendableResource{}
		res.On("Object").Return(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace}}, nil)
		res.On("Identity").Return("ConfigMap/" + cmName)
		res.On("DeleteOnSuspend").Return(true)
		res.On("Suspend").Return(nil)
		res.On("Mutate", mock.Anything).Return(nil)
		res.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil)

		Expect(suspendedComponent(res).Reconcile(ctx, newTestReconcileContext(owner))).To(Succeed())

		Expect(apierrors.IsNotFound(k8sClient.Get(ctx, key(), &corev1.ConfigMap{}))).To(BeTrue())
	})

	It("keeps an object without the owner's reference unchanged on suspension with DeleteOnSuspend", func() {
		createConfigMap(false)
		before := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key(), before)).To(Succeed())
		res := &MockSuspendableResource{}
		res.On("Object").Return(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace}}, nil)
		res.On("Identity").Return("ConfigMap/" + cmName)
		res.On("DeleteOnSuspend").Return(true)
		// No Suspend, Mutate or SuspensionStatus expectation: reaching any of them
		// means the suspension apply was about to adopt the user's object, after
		// which the delete would find it owned.
		comp := suspendedComponent(res)

		Expect(comp.Reconcile(ctx, newTestReconcileContext(owner))).To(Succeed())

		Expect(comp.GetCondition(owner).Reason).To(Equal(string(Suspended)))
		after := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key(), after)).To(Succeed())
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion), "a kept object must not be written")
	})

	It("keeps an ownerless object on suspension when the desired object still carries the owner's reference", func() {
		createConfigMap(false)
		before := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key(), before)).To(Succeed())
		// The desired object keeps the owner reference that an earlier apply set,
		// while the live object under the name is a user's replacement without it.
		desired := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace}}
		Expect(controllerutil.SetControllerReference(owner, desired, scheme.Scheme)).To(Succeed())
		res := &MockSuspendableResource{}
		res.On("Object").Return(desired, nil)
		res.On("Identity").Return("ConfigMap/" + cmName)
		res.On("DeleteOnSuspend").Return(true)
		comp := suspendedComponent(res)

		Expect(comp.Reconcile(ctx, newTestReconcileContext(owner))).To(Succeed())

		after := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key(), after)).To(Succeed())
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion), "a kept object must not be written")
	})

	It("keeps an object disowned between the suspension read and the suspension apply", func() {
		createConfigMap(true)
		res := &MockSuspendableResource{}
		res.On("Object").Return(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: namespace}}, nil)
		res.On("Identity").Return("ConfigMap/" + cmName)
		res.On("DeleteOnSuspend").Return(true)
		res.On("Suspend").Return(nil)
		res.On("Mutate", mock.Anything).Return(nil)
		res.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil)
		comp := suspendedComponent(res)

		base, err := client.NewWithWatch(cfg, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())
		disownBeforeApply := interceptor.NewClient(base, interceptor.Funcs{
			Patch: func(
				ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption,
			) error {
				if obj.GetName() == cmName {
					live := &corev1.ConfigMap{}
					Expect(c.Get(ctx, key(), live)).To(Succeed())
					live.OwnerReferences = nil
					Expect(c.Update(ctx, live)).To(Succeed())
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		})
		rec := newTestReconcileContext(owner)
		rec.Client = disownBeforeApply

		Expect(comp.Reconcile(ctx, rec)).To(Succeed())

		after := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, key(), after)).To(Succeed())
		Expect(after.OwnerReferences).To(BeEmpty(), "the suspension apply must not take over the disowned object")
		Expect(comp.GetCondition(owner).Reason).To(Equal(string(Suspending)))
	})

	It("reports Disabled for a disabled component feature gate whether or not the object is kept", func() {
		createConfigMap(false)
		comp, err := disabledComponentGate(desiredResource(), DeleteOnlyIfOwned())
		Expect(err).NotTo(HaveOccurred())

		Expect(comp.Reconcile(ctx, newTestReconcileContext(owner))).To(Succeed())

		Expect(comp.GetCondition(owner).Reason).To(Equal(string(Disabled)))
	})
})
