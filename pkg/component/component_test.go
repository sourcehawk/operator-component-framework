package component

import (
	"context"
	"fmt"
	"time"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
			res.On("Object").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/test-cm")
			res.On("Mutate", mock.Anything).Return(nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

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
			Expect(cond.Reason).To(Equal(string(Healthy)))
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
			res.On("Object").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/test-alive-cm")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ConvergingStatus", concepts.ConvergingOperationCreated).Return(concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusCreating,
				Reason: "Waiting for creation",
			}, nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res}}
			comp.participationLookup = map[string]ParticipationMode{
				res.Identity(): ParticipationModeRequired,
			}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify status condition is False but Reason is Creating
			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(AliveCreating)))
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
			res.On("Object").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/test-readonly-cm")
			res.On("ConvergingStatus", concepts.ConvergingOperationNone).Return(concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusHealthy,
				Reason: "Read-only healthy",
			}, nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res, ReadOnly: true}}
			comp.participationLookup = map[string]ParticipationMode{
				res.Identity(): ParticipationModeRequired,
			}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify status condition
			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(Healthy)))
		})

		It("should NOT mutate or apply read-only resources", func() {
			// Given: a read-only resource that exists in the cluster
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-no-mutate-cm",
					Namespace: namespace,
				},
				Data: map[string]string{"original": "data"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			res := &MockResource{}
			res.On("Identity").Return("ConfigMap/test-no-mutate-cm")
			res.On("Object").Return(cm, nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res, ReadOnly: true}}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Mutate must never be called for read-only resources
			res.AssertNotCalled(GinkgoT(), "Mutate", mock.Anything)

			// Verify the cluster object was not modified
			var fetched corev1.ConfigMap
			Expect(k8sClient.Get(ctx, client.ObjectKey{
				Name: "test-no-mutate-cm", Namespace: namespace,
			}, &fetched)).To(Succeed())
			Expect(fetched.Data).To(Equal(map[string]string{"original": "data"}))
			Expect(fetched.OwnerReferences).To(BeEmpty(), "read-only resource should not get an owner reference")
		})

		It("should aggregate status across both create and read-only resources", func() {
			// Given
			cm1 := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "create-cm", Namespace: namespace},
			}
			res1 := &MockAliveResource{}
			res1.On("Object").Return(cm1, nil)
			res1.On("Identity").Return("ConfigMap/create-cm")
			res1.On("Mutate", mock.Anything).Return(nil)
			res1.On("ConvergingStatus", concepts.ConvergingOperationCreated).Return(concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusHealthy,
				Reason: "Creation healthy",
			}, nil)

			cm2 := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "read-cm", Namespace: namespace},
			}
			Expect(k8sClient.Create(ctx, cm2)).To(Succeed())

			res2 := &MockAliveResource{}
			res2.On("Object").Return(cm2, nil)
			res2.On("Identity").Return("ConfigMap/read-cm")
			res2.On("ConvergingStatus", concepts.ConvergingOperationNone).Return(concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusCreating, // Dominant status (False/Creating)
				Reason: "Read resource still preparing",
			}, nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res1}, {Resource: res2, ReadOnly: true}}
			comp.participationLookup = map[string]ParticipationMode{
				res1.Identity(): ParticipationModeRequired,
				res2.Identity(): ParticipationModeRequired,
			}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			// Verify status condition reflects the dominant "Creating" status from the read resource
			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(AliveCreating)))
			Expect(cond.Message).To(ContainSubstring("Read resource still preparing"))
		})

		It("should successfully reconcile an empty component to Ready", func() {
			// Given
			comp.reconcileResources = nil

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(Healthy)))
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
			createRes.On("Object").Return(cm, nil)
			createRes.On("Identity").Return("ConfigMap/should-not-be-created")
			createRes.On("Mutate", mock.Anything).Return(nil)

			// Set up suspendable resource
			suspendRes := &MockSuspendableResource{}
			suspendRes.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "suspend-me", Namespace: namespace},
			}, nil)
			suspendRes.On("Identity").Return("suspend-me")
			suspendRes.On("Suspend").Return(nil)
			suspendRes.On("Mutate", mock.Anything).Return(nil)
			suspendRes.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: "Suspended",
			}, nil)
			suspendRes.On("DeleteOnSuspend").Return(false)

			comp.reconcileResources = []reconcileEntry{{Resource: suspendRes}, {Resource: createRes}}

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
			res.On("Object").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/test-suspended-cm")
			res.On("Suspend").Return(nil)
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: "Suspended",
			}, nil)
			res.On("DeleteOnSuspend").Return(true)

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

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
			res.On("Object").Return(cm, nil)
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
			res.On("Object").Return(nil, fmt.Errorf("reconciliation error"))
			res.On("Identity").Return("failing-resource")

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

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
			res.On("Object").Return(nil, fmt.Errorf("read error"))
			res.On("Identity").Return("failing-read-resource")

			comp.reconcileResources = []reconcileEntry{{Resource: res, ReadOnly: true}}

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
			res.On("Object").Return(nil, fmt.Errorf("delete object error"))
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
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "failing-suspend-resource", Namespace: namespace},
			}, nil)
			res.On("DeleteOnSuspend").Return(false)
			res.On("Suspend").Return(fmt.Errorf("suspend error"))
			res.On("Identity").Return("failing-suspend-resource")

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

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
			susRes.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "suspend-ok", Namespace: namespace},
			}, nil)
			susRes.On("Identity").Return("suspend-ok")
			susRes.On("Suspend").Return(nil)
			susRes.On("Mutate", mock.Anything).Return(nil)
			susRes.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{Status: concepts.SuspensionStatusSuspended}, nil)
			susRes.On("DeleteOnSuspend").Return(false)

			// A resource that fails deletion
			delRes := &MockResource{}
			delRes.On("Object").Return(nil, fmt.Errorf("suspend-delete error"))
			delRes.On("Identity").Return("failing-suspend-delete-resource")

			comp.reconcileResources = []reconcileEntry{{Resource: susRes}}
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

	Describe("Participation Modes", func() {
		var resReq, resAux *MockAliveResource

		BeforeEach(func() {
			resReq = &MockAliveResource{}
			resReq.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "req-res", Namespace: namespace},
			}, nil)
			resReq.On("Identity").Return("ConfigMap/req-res")
			resReq.On("Mutate", mock.Anything).Return(nil)

			resAux = &MockAliveResource{}
			resAux.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "aux-res", Namespace: namespace},
			}, nil)
			resAux.On("Identity").Return("ConfigMap/aux-res")
			resAux.On("Mutate", mock.Anything).Return(nil)
		})

		reconcileAndCheck := func(rReq, rAux *MockAliveResource, rReqStatus, rAuxStatus concepts.AliveConvergingStatus, expectedStatus metav1.ConditionStatus, expectedReason string) {
			// Given
			rReq.On("ConvergingStatus", mock.Anything).Return(concepts.AliveStatusWithReason{
				Status: rReqStatus,
				Reason: string(rReqStatus),
			}, nil)

			rAux.On("ConvergingStatus", mock.Anything).Return(concepts.AliveStatusWithReason{
				Status: rAuxStatus,
				Reason: string(rAuxStatus),
			}, nil)

			c, err := NewComponentBuilder().
				WithName("test-comp").
				WithConditionType("Ready").
				WithResource(rReq, ResourceOptions{ParticipationMode: ParticipationModeRequired}).
				WithResource(rAux, ResourceOptions{ParticipationMode: ParticipationModeAuxiliary}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			// When
			err = c.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			cond := c.GetCondition(owner)
			Expect(cond.Status).To(Equal(expectedStatus))
			Expect(cond.Reason).To(Equal(expectedReason))
		}

		It("should ignore health of auxiliary resources for aggregation", func() {
			reconcileAndCheck(resReq, resAux, concepts.AliveConvergingStatusHealthy, concepts.AliveConvergingStatusFailing, metav1.ConditionTrue, string(Healthy))
		})

		It("should consider health of required resources for aggregation", func() {
			reconcileAndCheck(resReq, resAux, concepts.AliveConvergingStatusFailing, concepts.AliveConvergingStatusHealthy, metav1.ConditionFalse, string(AliveFailing))
		})

		It("should use ParticipationModeRequired as default for all resource types", func() {
			// Given: an Alive resource that is healthy and a Completable resource that is still running.
			// Both default to Required, so the still-running Completable should block the component.
			resAlive := &MockAliveResource{}
			resAlive.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "alive-res", Namespace: namespace},
			}, nil)
			resAlive.On("Identity").Return("ConfigMap/alive-res")
			resAlive.On("Mutate", mock.Anything).Return(nil)
			resAlive.On("ConvergingStatus", mock.Anything).Return(concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusHealthy,
				Reason: "Healthy",
			}, nil)

			resComp := &MockCompletableResource{}
			resComp.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "comp-res", Namespace: namespace},
			}, nil)
			resComp.On("Identity").Return("ConfigMap/comp-res")
			resComp.On("Mutate", mock.Anything).Return(nil)
			// Completable is Required by default, so a running task should block the component
			resComp.On("ConvergingStatus", mock.Anything).Return(concepts.CompletionStatusWithReason{
				Status: concepts.CompletionStatusRunning,
				Reason: "Running",
			}, nil)

			c, err := NewComponentBuilder().
				WithName("test-comp").
				WithConditionType("Ready").
				WithResource(resAlive, ResourceOptions{}). // Default mode (Required)
				WithResource(resComp, ResourceOptions{}).  // Default mode (Required)
				Build()
			Expect(err).NotTo(HaveOccurred())

			// When
			err = c.Reconcile(ctx, recCtx)

			// Then: component is not ready because the Completable resource is still running
			Expect(err).NotTo(HaveOccurred())
			cond := c.GetCondition(owner)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(CompletionRunning)))

			// When both resources are healthy/completed, the component becomes Ready
			resAlive.ExpectedCalls = nil
			resComp.ExpectedCalls = nil

			resAlive.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "alive-res", Namespace: namespace},
			}, nil)
			resAlive.On("Identity").Return("ConfigMap/alive-res")
			resAlive.On("Mutate", mock.Anything).Return(nil)
			resAlive.On("ConvergingStatus", mock.Anything).Return(concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusHealthy,
				Reason: "Healthy",
			}, nil)

			resComp.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "comp-res", Namespace: namespace},
			}, nil)
			resComp.On("Identity").Return("ConfigMap/comp-res")
			resComp.On("Mutate", mock.Anything).Return(nil)
			resComp.On("ConvergingStatus", mock.Anything).Return(concepts.CompletionStatusWithReason{
				Status: concepts.CompletionStatusCompleted,
				Reason: "Completed",
			}, nil)

			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())
			cond = c.GetCondition(owner)
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Describe("Reconciliation with Concepts", func() {
		DescribeTable("should handle resource concepts",
			func(res Resource, name string, status any) {
				var m *mock.Mock
				switch r := res.(type) {
				case *MockAliveResource:
					m = &r.Mock
				case *MockCompletableResource:
					m = &r.Mock
				case *MockOperationalResource:
					m = &r.Mock
				}

				m.On("Object").Return(&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: name + "-res", Namespace: namespace},
				}, nil)
				m.On("Identity").Return("ConfigMap/" + name + "-res")
				m.On("Mutate", mock.Anything).Return(nil)
				m.On("ConvergingStatus", concepts.ConvergingOperationCreated).Return(status, nil)

				c, _ := NewComponentBuilder().
					WithName(name+"-comp").
					WithConditionType("Ready").
					WithResource(res, ResourceOptions{
						ParticipationMode: ParticipationModeRequired,
					}).
					Build()

				err := c.Reconcile(ctx, recCtx)
				Expect(err).NotTo(HaveOccurred())

				condition := c.GetCondition(owner)
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			},
			Entry("Alive resources", &MockAliveResource{}, "alive", concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusHealthy,
				Reason: "All good",
			}),
			Entry("Completable resources", &MockCompletableResource{}, "complete", concepts.CompletionStatusWithReason{
				Status: concepts.CompletionStatusCompleted,
				Reason: "Job done",
			}),
			Entry("Operational resources", &MockOperationalResource{}, "op", concepts.OperationalStatusWithReason{
				Status: concepts.OperationalStatusOperational,
				Reason: "Operational",
			}),
		)

		It("should handle Graceful resources - Degraded after grace period", func() {
			res := &MockAliveResource{} // MockAliveResource also implements Graceful in zz_mock
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "grace-res", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/grace-res")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ConvergingStatus", mock.Anything).Return(concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusFailing,
				Reason: "Still failing",
			}, nil)
			res.On("GraceStatus").Return(concepts.GraceStatusWithReason{
				Status: concepts.GraceStatusDegraded,
				Reason: "Degraded but partially functional",
			}, nil)

			// Set a VERY short grace period
			gracePeriod := 1 * time.Nanosecond
			c, _ := NewComponentBuilder().
				WithName("grace-comp").
				WithConditionType("Ready").
				WithGracePeriod(gracePeriod).
				WithResource(res, ResourceOptions{
					ParticipationMode: ParticipationModeRequired,
				}).
				Build()

			// 1. Initial reconcile to set the condition and its transition time
			err := c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			// 2. Wait for grace period to expire
			time.Sleep(10 * time.Millisecond)

			// 3. Reconcile again, now graceExpired should be true
			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			condition := c.GetCondition(owner)
			Expect(condition.Status).To(Equal(metav1.ConditionFalse))
			Expect(condition.Reason).To(Equal(string(concepts.GraceStatusDegraded)))
			Expect(condition.Message).To(ContainSubstring("Degraded but partially functional"))
		})

		It("should handle Suspendable resources", func() {
			res := &MockSuspendableResource{}
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "sus-res", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/sus-res")
			res.On("Suspend").Return(nil)
			res.On("Mutate", mock.Anything).Return(nil) // Added to fix panic
			res.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: "Stopped",
			}, nil)
			res.On("DeleteOnSuspend").Return(false)

			c, _ := NewComponentBuilder().
				WithName("sus-comp").
				WithConditionType("Ready").
				WithResource(res, ResourceOptions{
					ParticipationMode: ParticipationModeRequired,
				}).
				Suspend(true).
				Build()

			err := c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			condition := c.GetCondition(owner)
			Expect(condition.Status).To(Equal(metav1.ConditionTrue)) // Suspend status is true when suspended
			Expect(condition.Reason).To(Equal("Suspended"))
		})
	})

	Context("Data Extraction", func() {
		It("should execute extraction during normal reconcile flow", func() {
			// Given
			res := &MockExtractableResource{}
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/test-cm")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ExtractData").Return(nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

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

			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/test-cm")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ExtractData").Return(nil)

			// It also needs to be Suspendable to not fail in suspendResources if it's in createResources
			// Actually, suspendResources handles non-suspendable resources as already suspended.
			// Let's check suspend_test.go to be sure.

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			res.AssertNotCalled(GinkgoT(), "ExtractData")
		})

		It("should propagate extraction errors and set status to Error", func() {
			// Given
			res := &MockExtractableResource{}
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/test-cm")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ExtractData").Return(fmt.Errorf("extraction failed"))

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

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
			res1.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "res1", Namespace: namespace},
			}, nil)
			res1.On("Identity").Return("ConfigMap/res1")
			res1.On("Mutate", mock.Anything).Return(nil)
			res1.On("ExtractData").Return(nil)

			res2 := &MockResource{}
			res2.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "res2", Namespace: namespace},
			}, nil)
			res2.On("Identity").Return("ConfigMap/res2")
			res2.On("Mutate", mock.Anything).Return(nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res1}, {Resource: res2}}

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
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "ext-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/ext-cm")
			res.On("Mutate", mock.Anything).Return(nil)

			c, err := NewComponentBuilder().
				WithName("ext-comp").
				WithConditionType("ExtReady").
				WithResource(res, ResourceOptions{}).
				Build()

			Expect(err).NotTo(HaveOccurred())

			// When
			err = c.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(extracted).To(BeTrue(), "Extraction logic should have been invoked")
		})
	})

	Context("Feature Gate", func() {
		It("should delete all resources and set condition to Disabled when gate is disabled", func() {
			// Given: pre-create a resource so we can verify deletion
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "gated-cm", Namespace: namespace},
				Data:       map[string]string{"key": "value"},
			}
			Expect(k8sClient.Create(ctx, cm)).To(Succeed())

			res := &MockResource{}
			res.On("Object").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/gated-cm")

			gate := &testGate{enabled: false}
			c, err := NewComponentBuilder().
				WithName("gate-test").
				WithConditionType("TestComponentReady").
				WithFeatureGate(gate).
				WithResource(res, ResourceOptions{}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			// When
			err = c.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(Disabled)))

			// Verify resource was deleted
			deletedCm := &corev1.ConfigMap{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "gated-cm", Namespace: namespace}, deletedCm)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})

		It("should reconcile normally when gate is enabled", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "enabled-gate-cm", Namespace: namespace},
				Data:       map[string]string{"key": "value"},
			}

			res := &MockResource{}
			res.On("Object").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/enabled-gate-cm")
			res.On("Mutate", mock.Anything).Return(nil)

			gate := &testGate{enabled: true}
			c, err := NewComponentBuilder().
				WithName("gate-enabled-test").
				WithConditionType("TestComponentReady").
				WithFeatureGate(gate).
				WithResource(res, ResourceOptions{}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(Healthy)))
		})

		It("should take precedence over suspension when gate is disabled", func() {
			res := &MockSuspendableResource{}
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "gate-sus-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/gate-sus-cm")

			gate := &testGate{enabled: false}
			c, err := NewComponentBuilder().
				WithName("gate-sus-test").
				WithConditionType("TestComponentReady").
				WithFeatureGate(gate).
				WithResource(res, ResourceOptions{}).
				Suspend(true).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(Disabled)))
		})

		It("should set condition to FeatureGateError when gate evaluation fails", func() {
			gate := &testGate{err: fmt.Errorf("gate check failed")}
			c, err := NewComponentBuilder().
				WithName("gate-err-test").
				WithConditionType("TestComponentReady").
				WithFeatureGate(gate).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).To(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(FeatureGateError)))
		})

		It("should handle deletion of non-existent resources without error", func() {
			// Resource doesn't exist in the cluster
			res := &MockResource{}
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "nonexistent-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/nonexistent-cm")

			gate := &testGate{enabled: false}
			c, err := NewComponentBuilder().
				WithName("gate-noexist-test").
				WithConditionType("TestComponentReady").
				WithFeatureGate(gate).
				WithResource(res, ResourceOptions{}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Disabled)))
		})
	})

	Context("Prerequisite", func() {
		It("should block reconciliation when prerequisite is not met", func() {
			res := &MockResource{}
			res.On("Identity").Return("ConfigMap/prereq-cm")

			prereq := &testPrereq{met: false, reason: "dependency not ready"}
			c, err := NewComponentBuilder().
				WithName("prereq-test").
				WithConditionType("TestComponentReady").
				WithPrerequisite(prereq).
				WithResource(res, ResourceOptions{}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(PrerequisiteNotMet)))
			Expect(cond.Message).To(Equal("dependency not ready"))

			// Verify resource was NOT processed
			res.AssertNotCalled(GinkgoT(), "Object")
		})

		It("should reconcile normally when prerequisite is met", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "prereq-met-cm", Namespace: namespace},
				Data:       map[string]string{"key": "value"},
			}
			res := &MockResource{}
			res.On("Object").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/prereq-met-cm")
			res.On("Mutate", mock.Anything).Return(nil)

			prereq := &testPrereq{met: true}
			c, err := NewComponentBuilder().
				WithName("prereq-met-test").
				WithConditionType("TestComponentReady").
				WithPrerequisite(prereq).
				WithResource(res, ResourceOptions{}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
			Expect(cond.Reason).To(Equal(string(Healthy)))
		})

		It("should not re-evaluate prerequisite after barrier has passed", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "barrier-pass-cm", Namespace: namespace},
				Data:       map[string]string{"key": "value"},
			}
			res := &MockResource{}
			res.On("Object").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/barrier-pass-cm")
			res.On("Mutate", mock.Anything).Return(nil)

			callCount := 0
			prereq := &testPrereqFunc{
				checkFn: func() (PrerequisiteResult, error) {
					callCount++
					return PrerequisiteResult{Status: PrerequisiteStatusMet}, nil
				},
			}

			c, err := NewComponentBuilder().
				WithName("barrier-pass-test").
				WithConditionType("TestComponentReady").
				WithPrerequisite(prereq).
				WithResource(res, ResourceOptions{}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			// First reconcile: prerequisite is checked and passes
			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(1))

			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Healthy)))

			// Second reconcile: barrier should be passed, prerequisite should NOT be re-evaluated
			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(1), "Prerequisite should not have been re-evaluated")
		})

		It("should block suspension while prerequisite has not passed", func() {
			res := &MockSuspendableResource{}
			res.On("Identity").Return("ConfigMap/prereq-sus-cm")

			prereq := &testPrereq{met: false, reason: "dependency not ready"}
			c, err := NewComponentBuilder().
				WithName("prereq-sus-test").
				WithConditionType("TestComponentReady").
				WithPrerequisite(prereq).
				WithResource(res, ResourceOptions{}).
				Suspend(true).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(PrerequisiteNotMet)))
			// Suspension methods should not have been called
			res.AssertNotCalled(GinkgoT(), "Suspend")
		})

		It("should allow suspension after prerequisite has passed", func() {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "prereq-then-sus", Namespace: namespace},
			}
			res := &MockSuspendableResource{}
			res.On("Object").Return(cm, nil)
			res.On("Identity").Return("ConfigMap/prereq-then-sus")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("Suspend").Return(nil)
			res.On("SuspensionStatus").Return(concepts.SuspensionStatusWithReason{
				Status: concepts.SuspensionStatusSuspended,
				Reason: "Stopped",
			}, nil)
			res.On("DeleteOnSuspend").Return(false)

			prereq := &testPrereq{met: true}
			c, err := NewComponentBuilder().
				WithName("prereq-then-sus-test").
				WithConditionType("TestComponentReady").
				WithPrerequisite(prereq).
				WithResource(res, ResourceOptions{}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			// First reconcile: prerequisite passes, normal reconciliation
			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())
			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Healthy)))

			// Second reconcile: suspended, barrier already passed so suspension proceeds
			c.suspended = true
			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())
			cond = getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Suspended)))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should handle gate disabled while prerequisite is not met", func() {
			res := &MockResource{}
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "gate-prereq-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("ConfigMap/gate-prereq-cm")

			gate := &testGate{enabled: false}
			prereq := &testPrereq{met: false, reason: "dependency not ready"}
			c, err := NewComponentBuilder().
				WithName("gate-prereq-test").
				WithConditionType("TestComponentReady").
				WithFeatureGate(gate).
				WithPrerequisite(prereq).
				WithResource(res, ResourceOptions{}).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			// Gate takes precedence
			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Disabled)))
			Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should set condition to PrerequisiteNotMet when prerequisite check fails", func() {
			prereq := &testPrereqFunc{
				checkFn: func() (PrerequisiteResult, error) {
					return PrerequisiteResult{}, fmt.Errorf("prereq check failed")
				},
			}
			c, err := NewComponentBuilder().
				WithName("prereq-err-test").
				WithConditionType("TestComponentReady").
				WithPrerequisite(prereq).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).To(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(PrerequisiteNotMet)))
		})

		It("should require all prerequisites to pass", func() {
			prereq1 := &testPrereq{met: true}
			prereq2 := &testPrereq{met: false, reason: "second dep not ready"}
			prereq3 := &testPrereq{met: true}

			c, err := NewComponentBuilder().
				WithName("multi-prereq-test").
				WithConditionType("TestComponentReady").
				WithPrerequisite(prereq1).
				WithPrerequisite(prereq2).
				WithPrerequisite(prereq3).
				Build()
			Expect(err).NotTo(HaveOccurred())

			err = c.Reconcile(ctx, recCtx)
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(PrerequisiteNotMet)))
			Expect(cond.Message).To(Equal("second dep not ready"))
		})
	})

	Context("Guards", func() {
		It("should apply resource normally when guard returns unblocked", func() {
			// Given
			res := &MockGuardableResource{}
			res.On("Identity").Return("v1/ConfigMap/unblocked-cm")
			res.On("GuardStatus").Return(concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusUnblocked,
			}, nil)
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "unblocked-cm", Namespace: namespace},
			}, nil)
			res.On("Mutate", mock.Anything).Return(nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			res.AssertCalled(GinkgoT(), "Object")
			res.AssertCalled(GinkgoT(), "Mutate", mock.Anything)
		})

		It("should block resource and set condition to Blocked when guard returns blocked", func() {
			// Given
			res := &MockGuardableAliveResource{}
			res.On("GuardStatus").Return(concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusBlocked,
				Reason: "waiting for cloud provider role ARN",
			}, nil)
			res.On("Identity").Return("v1/ConfigMap/guarded-cm")

			comp.reconcileResources = []reconcileEntry{{Resource: res}}
			comp.participationLookup = map[string]ParticipationMode{
				"v1/ConfigMap/guarded-cm": ParticipationModeRequired,
			}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			res.AssertNotCalled(GinkgoT(), "Object")
			res.AssertNotCalled(GinkgoT(), "Mutate", mock.Anything)

			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(GuardBlocked)))
			Expect(cond.Message).To(ContainSubstring("waiting for cloud provider role ARN"))
		})

		It("should skip all resources after a blocked guard", func() {
			// Given
			res1 := &MockResource{}
			res1.On("Identity").Return("v1/ConfigMap/first-cm")
			res1.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "first-cm", Namespace: namespace},
			}, nil)
			res1.On("Mutate", mock.Anything).Return(nil)

			res2 := &MockGuardableResource{}
			res2.On("Identity").Return("v1/ConfigMap/blocked-cm")
			res2.On("GuardStatus").Return(concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusBlocked,
				Reason: "blocked",
			}, nil)

			res3 := &MockResource{}

			comp.reconcileResources = []reconcileEntry{{Resource: res1}, {Resource: res2}, {Resource: res3}}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			res1.AssertCalled(GinkgoT(), "Object")
			res2.AssertNotCalled(GinkgoT(), "Object")
			res3.AssertNotCalled(GinkgoT(), "Object")
		})

		It("should set condition to Error when guard evaluation fails", func() {
			// Given
			res := &MockGuardableResource{}
			res.On("Identity").Return("v1/ConfigMap/guard-error")
			res.On("GuardStatus").Return(concepts.GuardStatusWithReason{}, fmt.Errorf("guard evaluation failed"))

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("guard evaluation failed"))

			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Error)))
		})

		It("should NOT evaluate guards during suspension", func() {
			// Given
			comp.suspended = true

			res := &MockGuardableResource{}
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "suspended-guarded-cm", Namespace: namespace},
			}, nil)
			res.On("Mutate", mock.Anything).Return(nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			res.AssertNotCalled(GinkgoT(), "GuardStatus")
		})

		It("should make extracted data available to a subsequent resource's guard and mutation", func() {
			// This validates the full extractor -> guard -> mutation flow:
			// Resource A's extractor populates a shared variable, Resource B's guard
			// reads it to unblock, and Resource B's mutation reads it to inject into the spec.
			var roleARN string

			res1 := &MockGuardableExtractableResource{}
			res1.On("Identity").Return("v1/ConfigMap/role")
			res1.On("GuardStatus").Return(concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusUnblocked,
			}, nil)
			res1.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "role-cm", Namespace: namespace},
			}, nil)
			res1.On("Mutate", mock.Anything).Return(nil)
			res1.On("ExtractData").Run(func(_ mock.Arguments) {
				roleARN = "arn:aws:iam::123456789:role/my-role"
			}).Return(nil)

			// Resource B: guard dynamically checks roleARN, mutation injects it into the object.
			var mutatedARN string
			res2 := &MockGuardableResource{}
			res2.On("Identity").Return("v1/ConfigMap/bucket")
			res2.On("GuardStatus").Return(concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusUnblocked,
			}, nil).Maybe()
			res2.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "bucket-cm", Namespace: namespace},
			}, nil)
			res2.On("Mutate", mock.Anything).Run(func(args mock.Arguments) {
				// The mutation reads roleARN at Mutate() time. Because per-resource
				// extraction ran after res1 was applied, roleARN is already populated.
				mutatedARN = roleARN
			}).Return(nil)

			comp.reconcileResources = []reconcileEntry{{Resource: res1}, {Resource: res2}}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(roleARN).To(Equal("arn:aws:iam::123456789:role/my-role"))
			Expect(mutatedARN).To(Equal("arn:aws:iam::123456789:role/my-role"),
				"mutation on resource B should see the value extracted from resource A")
		})

		It("should propagate extraction errors from per-resource extraction", func() {
			// Given
			res := &MockGuardableExtractableResource{}
			res.On("GuardStatus").Return(concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusUnblocked,
			}, nil)
			res.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "fail-extract-cm", Namespace: namespace},
			}, nil)
			res.On("Identity").Return("v1/ConfigMap/fail-extract")
			res.On("Mutate", mock.Anything).Return(nil)
			res.On("ExtractData").Return(fmt.Errorf("extraction failed"))

			comp.reconcileResources = []reconcileEntry{{Resource: res}}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("extraction failed"))

			cond := getOwnerCondition()
			Expect(cond.Reason).To(Equal(string(Error)))
		})

		It("should collect alive status alongside blocked guard results", func() {
			// Given: first resource is alive and healthy, second is blocked
			alive := &MockAliveResource{}
			alive.On("Object").Return(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "alive-cm", Namespace: namespace},
			}, nil)
			alive.On("Mutate", mock.Anything).Return(nil)
			alive.On("ConvergingStatus", mock.Anything).Return(concepts.AliveStatusWithReason{
				Status: concepts.AliveConvergingStatusHealthy,
				Reason: "Ready",
			}, nil)
			alive.On("Identity").Return("v1/ConfigMap/alive")

			guarded := &MockGuardableAliveResource{}
			guarded.On("GuardStatus").Return(concepts.GuardStatusWithReason{
				Status: concepts.GuardStatusBlocked,
				Reason: "waiting",
			}, nil)
			guarded.On("Identity").Return("v1/ConfigMap/guarded")

			comp.reconcileResources = []reconcileEntry{{Resource: alive}, {Resource: guarded}}
			comp.participationLookup = map[string]ParticipationMode{
				"v1/ConfigMap/alive":   ParticipationModeRequired,
				"v1/ConfigMap/guarded": ParticipationModeRequired,
			}

			// When
			err := comp.Reconcile(ctx, recCtx)

			// Then
			Expect(err).NotTo(HaveOccurred())

			cond := getOwnerCondition()
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal(string(GuardBlocked)))
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

type testGate struct {
	enabled bool
	err     error
}

func (g *testGate) Enabled() (bool, error) {
	return g.enabled, g.err
}

type testPrereq struct {
	met    bool
	reason string
}

func (p *testPrereq) Check(_ ReconcileContext) (PrerequisiteResult, error) {
	if p.met {
		return PrerequisiteResult{Status: PrerequisiteStatusMet}, nil
	}
	return PrerequisiteResult{Status: PrerequisiteStatusNotMet, Reason: p.reason}, nil
}

type testPrereqFunc struct {
	checkFn func() (PrerequisiteResult, error)
}

func (p *testPrereqFunc) Check(_ ReconcileContext) (PrerequisiteResult, error) {
	return p.checkFn()
}
