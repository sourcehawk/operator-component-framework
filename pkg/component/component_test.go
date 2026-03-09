package component

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2" //nolint:revive
	. "github.com/onsi/gomega"    //nolint:revive
	"github.com/stretchr/testify/mock"
)

var _ = Describe("Component Reconciler", func() {
	var (
		ctx       = context.Background()
		namespace string
		owner     *MockOperatorCRD
		comp      *Component
		recCtx    ReconcileContext
	)

	BeforeEach(func() {
		namespace = createNamespace(ctx, "component-test-")
		owner = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-owner",
				Namespace: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, owner)).To(Succeed())

		recCtx = newTestReconcileContext(owner)

		comp = &Component{
			name:          "test-component",
			conditionType: "TestComponentReady",
		}
	})

	getOwnerCondition := func() Condition {
		updatedOwner := &MockOperatorCRD{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: owner.Name, Namespace: namespace}, updatedOwner)).To(Succeed())
		return comp.GetCondition(updatedOwner)
	}

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, owner)).To(Succeed())
	})

	Context("Basic Reconciliation", func() {
		It("should return its name", func() {
			Expect(comp.GetName()).To(Equal("test-component"))
		})

		It("should return a synthetic Unknown condition if it does not exist on the owner", func() {
			// Given
			Expect(owner.Status.Conditions).To(BeEmpty())

			// When
			cond := comp.GetCondition(owner)

			// Then
			Expect(cond.ConditionType()).To(Equal(ConditionType("TestComponentReady")))
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(Unknown)))
		})

		It("should return an existing condition from the owner", func() {
			// Given
			existingCond := metav1.Condition{
				Type:               "TestComponentReady",
				Status:             metav1.ConditionTrue,
				Reason:             "ExistingReason",
				LastTransitionTime: metav1.Now(),
			}
			owner.Status.Conditions = []metav1.Condition{existingCond}

			// When
			cond := comp.GetCondition(owner)

			// Then
			Expect(string(cond.ConditionType())).To(Equal(existingCond.Type))
			Expect(cond.Status).To(Equal(existingCond.Status))
			Expect(cond.Reason).To(Equal(existingCond.Reason))
		})

		It("should successfully create resources and update status to Ready", func() {
			// Given
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cm",
					Namespace: namespace,
				},
				Data: map[string]string{"foo": "bar"},
			}
			res := &MockResource{}
			res.On("DesiredDefaultObject").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/test-cm")
			res.On("Mutate", mock.Anything).Return(nil)

			comp.createResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap exists
			createdCm := &corev1.ConfigMap{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "test-cm", Namespace: namespace}, createdCm)).To(Succeed())
			Expect(createdCm.Data).To(HaveKeyWithValue("foo", "bar"))
			Expect(createdCm.OwnerReferences).To(HaveLen(1))
			Expect(createdCm.OwnerReferences[0].Name).To(Equal(owner.Name))
			// Verify status condition
			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(Ready)))
		})

		It("should aggregate status from Alive resources", func() {
			// Given
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-alive-cm",
					Namespace: namespace,
				},
			}
			res := &MockAliveResource{}
			res.On("DesiredDefaultObject").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/test-alive-cm")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ConvergingStatus", ConvergingOperationCreated).Return(ConvergingStatusWithReason{
				Status: ConvergingStatusCreating,
				Reason: "Waiting for creation",
			}, nil)

			comp.createResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify status condition is False but Reason is Creating
			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(Creating)))
		})

		It("should handle read-only resources", func() {
			// Given
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-readonly-cm",
					Namespace: namespace,
				},
				Data: map[string]string{"read": "only"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			res := &MockAliveResource{}
			res.On("DesiredDefaultObject").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/test-readonly-cm")
			res.On("ConvergingStatus", ConvergingOperationNone).Return(ConvergingStatusWithReason{
				Status: ConvergingStatusReady,
				Reason: "Read-only ready",
			}, nil)

			comp.readResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify status condition
			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(Ready)))
		})

		It("should aggregate status across both create and read-only resources", func() {
			// Given
			cm1 := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "create-cm", Namespace: namespace},
			}
			res1 := &MockAliveResource{}
			res1.On("DesiredDefaultObject").Return(cm1, nil)
			res1.On("Identity").Return("ConfigMap/create-cm")
			res1.On("Mutate", mock.Anything).Return(nil)
			res1.On("ConvergingStatus", ConvergingOperationCreated).Return(ConvergingStatusWithReason{
				Status: ConvergingStatusReady,
				Reason: "Creation ready",
			}, nil)

			cm2 := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "read-cm", Namespace: namespace},
			}
			Expect(k8sClient.Create(ctx, cm2)).To(Succeed())

			res2 := &MockAliveResource{}
			res2.On("DesiredDefaultObject").Return(cm2, nil)
			res2.On("Identity").Return("ConfigMap/read-cm")
			res2.On("ConvergingStatus", ConvergingOperationNone).Return(ConvergingStatusWithReason{
				Status: ConvergingStatusCreating, // Dominant status (False/Creating)
				Reason: "Read resource still preparing",
			}, nil)

			comp.createResources = []Resource{res1}
			comp.readResources = []Resource{res2}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify status condition reflects the dominant "Creating" status from the read resource
			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(Creating)))
			Expect(cond.Message).To(ContainSubstring("Read resource still preparing"))
		})

		It("should successfully reconcile an empty component to Ready", func() {
			// Given
			comp.createResources = nil
			comp.readResources = nil

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(Ready)))
		})
	})

	Context("Suspension", func() {
		It("should take precedence over normal reconciliation", func() {
			// Given
			comp.suspended = true

			// Set up create resource that should NOT be created
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "should-not-be-created",
					Namespace: namespace,
				},
				Data: map[string]string{"foo": "bar"},
			}
			createRes := &MockResource{}
			createRes.On("DesiredDefaultObject").Return(cm, nil)
			createRes.On("Identity").Return("ConfigMap/should-not-be-created")
			createRes.On("Mutate", mock.Anything).Return(nil)

			// Set up suspendable resource
			suspendRes := &MockSuspendableResource{}
			suspendRes.On("DesiredDefaultObject").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "suspend-me", Namespace: namespace},
			}, nil)
			suspendRes.On("Identity").Return("suspend-me")
			suspendRes.On("Suspend").Return(nil)
			suspendRes.On("Mutate", mock.Anything).Return(nil)
			suspendRes.On("SuspensionStatus").Return(SuspensionStatusWithReason{
				Status: SuspensionStatusSuspended,
				Reason: "Suspended",
			}, nil)
			suspendRes.On("DeleteOnSuspend").Return(false)

			comp.createResources = []Resource{suspendRes, createRes}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap was NOT created (even though it's in createResources)
			createdCm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "should-not-be-created", Namespace: namespace}, createdCm)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())

			// Verify status condition
			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Suspended)))
		})

		It("should handle suspension and delete resources if needed", func() {
			// Given
			comp.suspended = true
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-suspended-cm",
					Namespace: namespace,
				},
			}
			// Pre-create the CM
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			res := &MockSuspendableResource{}
			res.On("DesiredDefaultObject").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/test-suspended-cm")
			res.On("Suspend").Return(nil)
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("SuspensionStatus").Return(SuspensionStatusWithReason{
				Status: SuspensionStatusSuspended,
				Reason: "Suspended",
			}, nil)
			res.On("DeleteOnSuspend").Return(true)

			comp.createResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify status condition reflects suspension
			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Suspended)))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))

			// Verify CM is deleted because DeleteOnSuspend was true
			deletedCm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test-suspended-cm", Namespace: namespace}, deletedCm)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})
	})

	Context("Deletion", func() {
		It("should delete resources registered for deletion", func() {
			// Given
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "to-be-deleted",
					Namespace: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			res := &MockResource{}
			res.On("DesiredDefaultObject").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/to-be-deleted")

			comp.deleteResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap is gone
			deletedCm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "to-be-deleted", Namespace: namespace}, deletedCm)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})
	})

	Context("Error Handling", func() {
		It("should update status to Error when reconciliation fails", func() {
			// Given
			res := &MockResource{}
			res.On("DesiredDefaultObject").Return(nil, fmt.Errorf("reconciliation error"))
			res.On("Identity").Return("failing-resource")

			comp.createResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("reconciliation error"))

			// Verify status condition is Error
			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(Error)))
			Expect(cond.Message).To(ContainSubstring("reconciliation error"))
		})

		It("should handle errors during read-only resource retrieval", func() {
			// Given
			res := &MockResource{}
			res.On("DesiredDefaultObject").Return(nil, fmt.Errorf("read error"))
			res.On("Identity").Return("failing-read-resource")

			comp.readResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("read error"))

			// Verify status condition is Error
			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Error)))
			Expect(cond.Message).To(ContainSubstring("read error"))
		})

		It("should handle errors during resource deletion in normal flow", func() {
			// Given
			res := &MockResource{}
			res.On("DesiredDefaultObject").Return(nil, fmt.Errorf("delete object error"))
			res.On("Identity").Return("failing-delete-resource")

			comp.deleteResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("delete object error"))

			// Verify status condition is Error
			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Error)))
			Expect(cond.Message).To(ContainSubstring("delete object error"))
		})

		It("should handle errors during suspension", func() {
			// Given
			comp.suspended = true
			res := &MockSuspendableResource{}
			res.On("Suspend").Return(fmt.Errorf("suspend error"))
			res.On("Identity").Return("failing-suspend-resource")

			comp.createResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("suspend error"))

			// Verify status condition is Error
			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Error)))
			Expect(cond.Message).To(ContainSubstring("suspend error"))
		})

		It("should handle errors during deletion in suspended flow", func() {
			// Given
			comp.suspended = true
			// A resource that suspends successfully
			susRes := &MockSuspendableResource{}
			susRes.On("DesiredDefaultObject").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "suspend-ok", Namespace: namespace},
			}, nil)
			susRes.On("Identity").Return("suspend-ok")
			susRes.On("Suspend").Return(nil)
			susRes.On("Mutate", mock.Anything).Return(nil)
			susRes.On("SuspensionStatus").Return(SuspensionStatusWithReason{Status: SuspensionStatusSuspended}, nil)
			susRes.On("DeleteOnSuspend").Return(false)

			// A resource that fails deletion
			delRes := &MockResource{}
			delRes.On("DesiredDefaultObject").Return(nil, fmt.Errorf("suspend-delete error"))
			delRes.On("Identity").Return("failing-suspend-delete-resource")

			comp.createResources = []Resource{susRes}
			comp.deleteResources = []Resource{delRes}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("suspend-delete error"))

			// Verify status condition is Error
			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Error)))
			Expect(cond.Message).To(ContainSubstring("suspend-delete error"))
		})
	})

	Context("Data Extraction", func() {
		It("should execute extraction during normal reconcile flow", func() {
			// Given
			res := &MockExtractableResource{}
			res.On("DesiredDefaultObject").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/test-cm")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ExtractData").Return(nil)

			comp.createResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			res.AssertCalled(GinkgoT(), "ExtractData")
		})

		It("should NOT execute extraction during suspended reconcile flow", func() {
			// Given
			comp.suspended = true
			res := &MockExtractableResource{}
			// We need to implement Suspendable for it to be processed in suspendResources
			// but we can just use a MockSuspendableResource that also implements DataExtractable
			// Wait, MockExtractableResource doesn't implement Suspendable.
			// Let's create a combined mock or just use MockExtractableResource and see if it's called.
			// Reconcile checks c.suspended FIRST.

			res.On("DesiredDefaultObject").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/test-cm")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ExtractData").Return(nil)

			// It also needs to be Suspendable to not fail in suspendResources if it's in createResources
			// Actually, suspendResources handles non-suspendable resources as already suspended.
			// Let's check suspend_test.go to be sure.

			comp.createResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			res.AssertNotCalled(GinkgoT(), "ExtractData")
		})

		It("should propagate extraction errors and set status to Error", func() {
			// Given
			res := &MockExtractableResource{}
			res.On("DesiredDefaultObject").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/test-cm")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ExtractData").Return(fmt.Errorf("extraction failed"))

			comp.createResources = []Resource{res}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("extraction failed"))

			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Error)))
			Expect(cond.Message).To(ContainSubstring("extraction failed"))
		})

		It("should handle multiple resources with and without DataExtractable", func() {
			// Given
			res1 := &MockExtractableResource{}
			res1.On("DesiredDefaultObject").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "res1", Namespace: namespace},
			}, nil)
			res1.On("Identity").Return("ConfigMap/res1")
			res1.On("Mutate", mock.Anything).Return(nil)
			res1.On("ExtractData").Return(nil)

			res2 := &MockResource{}
			res2.On("DesiredDefaultObject").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "res2", Namespace: namespace},
			}, nil)
			res2.On("Identity").Return("ConfigMap/res2")
			res2.On("Mutate", mock.Anything).Return(nil)

			comp.createResources = []Resource{res1, res2}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			res1.AssertCalled(GinkgoT(), "ExtractData")
		})
	})

	Context("Integration-style Data Extraction", func() {
		It("should invoke extraction logic when resource is reconciled through Component.Reconcile", func() {
			// Given
			extracted := false
			res := &testExtractableResource{
				MockResource: MockResource{},
				extractFn: func() error {
					extracted = true
					return nil
				},
			}
			res.On("DesiredDefaultObject").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "ext-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/ext-cm")
			res.On("Mutate", mock.Anything).Return(nil)

			comp := NewComponentBuilder(false).
				WithName("ext-comp").
				WithConditionType(ConditionType("ExtReady"))
			c, err := comp.WithResource(res, false, false).
				Build()
			Expect(err).NotTo(HaveOccurred())

			// When
			err = c.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(extracted).To(BeTrue(), "Extraction logic should have been invoked")
		})
	})
})

type testExtractableResource struct {
	MockResource

	extractFn func() error
}

func (t *testExtractableResource) ExtractData() error {
	return t.extractFn()
}
