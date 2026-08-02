package component

import (
	"context"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/sourcehawk/operator-component-framework/pkg/generic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// noopMutator satisfies generic.FeatureMutator for test resources that need
// no mutation behavior.
type noopMutator struct{}

func (*noopMutator) Apply() error { return nil }
func (*noopMutator) NextFeature() {}

// fakeCellProducer is a managed Resource producing one string cell. Its
// extraction records whether the cell was already set when extraction ran,
// which is how the reconcile-start reset is observed.
type fakeCellProducer struct {
	obj          *corev1.ConfigMap
	cell         *concepts.Data[string]
	setAtExtract []bool
}

func (f *fakeCellProducer) Identity() string {
	return "v1/ConfigMap/" + f.obj.Namespace + "/" + f.obj.Name
}
func (f *fakeCellProducer) Object() (client.Object, error) { return f.obj.DeepCopy(), nil }
func (f *fakeCellProducer) Mutate(client.Object) error     { return nil }
func (f *fakeCellProducer) ExtractData() error {
	f.setAtExtract = append(f.setAtExtract, f.cell.IsSet())
	f.cell.Set(f.obj.Data["db-host"])
	return nil
}
func (f *fakeCellProducer) ProducedData() []concepts.DataCell {
	return []concepts.DataCell{f.cell}
}

// suspendableCellProducer is a fakeCellProducer that also satisfies
// concepts.Suspendable with the no-op behavior a static managed resource has,
// so it takes part in the suspension path.
type suspendableCellProducer struct {
	*fakeCellProducer
}

func (*suspendableCellProducer) DeleteOnSuspend() bool { return false }
func (*suspendableCellProducer) Suspend() error        { return nil }
func (*suspendableCellProducer) SuspensionStatus() (concepts.SuspensionStatusWithReason, error) {
	return concepts.SuspensionStatusWithReason{
		Status: concepts.SuspensionStatusSuspended,
		Reason: "static resource is always suspended",
	}, nil
}

// silentCellProducer declares production of a cell but has no extraction, so
// the cell stays unset. It stands in for a producer whose extraction has not
// run yet (for example an absent read-only source).
type silentCellProducer struct {
	obj  *corev1.ConfigMap
	cell *concepts.Data[string]
}

func (f *silentCellProducer) Identity() string {
	return "v1/ConfigMap/" + f.obj.Namespace + "/" + f.obj.Name
}
func (f *silentCellProducer) Object() (client.Object, error) { return f.obj.DeepCopy(), nil }
func (f *silentCellProducer) Mutate(client.Object) error     { return nil }
func (f *silentCellProducer) ProducedData() []concepts.DataCell {
	return []concepts.DataCell{f.cell}
}

func newGuardedConsumer(ns string, cell *concepts.Data[string], optional bool) Resource {
	cm := &corev1.ConfigMap{}
	cm.Name = "consumer"
	cm.Namespace = ns
	b := generic.NewStaticBuilder[*corev1.ConfigMap, *noopMutator](
		cm,
		func(c *corev1.ConfigMap) string { return "v1/ConfigMap/" + c.Namespace + "/" + c.Name },
		func(*corev1.ConfigMap) *noopMutator { return &noopMutator{} },
	)
	if optional {
		b.WithOptionalData(cell)
	} else {
		b.WithDataGuard(cell)
	}
	res, err := b.Build()
	Expect(err).NotTo(HaveOccurred())
	return res
}

// cellMutator is a generic.FeatureMutator over a ConfigMap. It keeps the
// object being mutated so a test mutation can write extracted data into it.
type cellMutator struct {
	cm *corev1.ConfigMap
}

func (*cellMutator) Apply() error { return nil }
func (*cellMutator) NextFeature() {}

// newRequiringConsumer builds a managed ConfigMap that declares a data guard on
// cell and whose content mutation copies the cell's required value into the
// object. The mutation fails unless the cell was extracted earlier in the pass.
func newRequiringConsumer(ns string, cell *concepts.Data[string]) Resource {
	cm := &corev1.ConfigMap{}
	cm.Name = "consumer"
	cm.Namespace = ns
	b := generic.NewStaticBuilder[*corev1.ConfigMap, *cellMutator](
		cm,
		func(c *corev1.ConfigMap) string { return "v1/ConfigMap/" + c.Namespace + "/" + c.Name },
		func(c *corev1.ConfigMap) *cellMutator { return &cellMutator{cm: c} },
	)
	b.WithDataGuard(cell)
	b.WithMutation(generic.Mutation[*cellMutator]{
		Name: "copy-db-host",
		Mutate: func(m *cellMutator) error {
			value, err := cell.Require()
			if err != nil {
				return err
			}
			if m.cm.Data == nil {
				m.cm.Data = map[string]string{}
			}
			m.cm.Data["db-host"] = value
			return nil
		},
	})
	res, err := b.Build()
	Expect(err).NotTo(HaveOccurred())
	return res
}

var _ = Describe("Declared data reconciliation", func() {
	var (
		ctx       = context.Background()
		namespace string
		owner     *MockOperatorCRD
		recCtx    ReconcileContext
	)

	BeforeEach(func() {
		namespace = createNamespace(ctx, "data-reconcile-test-")
		owner = &MockOperatorCRD{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-owner",
				Namespace: namespace,
			},
		}
		Expect(k8sClient.Create(ctx, owner)).To(Succeed())

		recCtx = newTestReconcileContext(owner)
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, owner)).To(Succeed())
	})

	It("clears declared cells at the start of each reconcile", func() {
		cell := concepts.NewData[string]("db-host")
		producer := &fakeCellProducer{
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "producer", Namespace: namespace},
				Data:       map[string]string{"db-host": "postgres"},
			},
			cell: cell,
		}

		comp, err := NewComponentBuilder().
			WithName("data-reconcile-test").
			WithConditionType("DataReady").
			WithResource(producer).
			Build()
		Expect(err).NotTo(HaveOccurred())

		// First reconcile: the cell starts unset, so extraction observes false.
		Expect(comp.Reconcile(ctx, recCtx)).To(Succeed())
		Expect(producer.setAtExtract).To(Equal([]bool{false}))
		Expect(cell.IsSet()).To(BeTrue())

		// Second reconcile: without the reconcile-start reset, the cell would
		// still be set from the previous pass. The second recorded false proves
		// Reconcile cleared it before extraction ran.
		Expect(comp.Reconcile(ctx, recCtx)).To(Succeed())
		Expect(producer.setAtExtract).To(Equal([]bool{false, false}))
	})

	It("blocks a guarded consumer with the generated reason and surfaces it on the condition", func() {
		cell := concepts.NewData[string]("db-host")
		producer := &silentCellProducer{
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "producer", Namespace: namespace},
			},
			cell: cell,
		}
		consumer := newGuardedConsumer(namespace, cell, false)

		comp, err := NewComponentBuilder().
			WithName("data-reconcile-test").
			WithConditionType("DataReady").
			WithResource(producer).
			WithResource(consumer).
			Build()
		Expect(err).NotTo(HaveOccurred())

		Expect(comp.Reconcile(ctx, recCtx)).To(Succeed())

		cond := comp.GetCondition(owner)
		Expect(cond.Reason).To(Equal(string(GuardBlocked)))
		Expect(cond.Message).To(ContainSubstring(`waiting for data "db-host"`))

		// The guarded consumer must never have been created in the cluster.
		var fetched corev1.ConfigMap
		err = k8sClient.Get(ctx, client.ObjectKey{Name: "consumer", Namespace: namespace}, &fetched)
		Expect(err).To(HaveOccurred())
		Expect(client.IgnoreNotFound(err)).To(Succeed())
	})

	It("runs declared extractions while suspended so a requiring consumer still applies", func() {
		cell := concepts.NewData[string]("db-host")
		producer := &suspendableCellProducer{
			fakeCellProducer: &fakeCellProducer{
				obj: &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "producer", Namespace: namespace},
					Data:       map[string]string{"db-host": "postgres"},
				},
				cell: cell,
			},
		}
		consumer := newRequiringConsumer(namespace, cell)

		comp, err := NewComponentBuilder().
			WithName("data-reconcile-test").
			WithConditionType("DataReady").
			WithResource(producer).
			WithResource(consumer).
			Suspend(true).
			Build()
		Expect(err).NotTo(HaveOccurred())

		Expect(comp.Reconcile(ctx, recCtx)).To(Succeed())
		Expect(cell.IsSet()).To(BeTrue())

		// The consumer's Require-based mutation could only succeed because the
		// managed producer's extraction ran on the suspension path.
		var fetched corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "consumer", Namespace: namespace}, &fetched)).To(Succeed())
		Expect(fetched.Data).To(HaveKeyWithValue("db-host", "postgres"))

		cond := comp.GetCondition(owner)
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
		Expect(cond.Reason).To(Equal(string(Suspended)))
	})

	It("applies an optional consumer even when the cell is unset", func() {
		cell := concepts.NewData[string]("db-host")
		producer := &silentCellProducer{
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "producer", Namespace: namespace},
			},
			cell: cell,
		}
		consumer := newGuardedConsumer(namespace, cell, true)

		comp, err := NewComponentBuilder().
			WithName("data-reconcile-test").
			WithConditionType("DataReady").
			WithResource(producer).
			WithResource(consumer).
			Build()
		Expect(err).NotTo(HaveOccurred())

		Expect(comp.Reconcile(ctx, recCtx)).To(Succeed())

		// The optional consumer must have been created despite the unset cell.
		var fetched corev1.ConfigMap
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "consumer", Namespace: namespace}, &fetched)).To(Succeed())

		cond := comp.GetCondition(owner)
		Expect(cond.Reason).NotTo(Equal(string(GuardBlocked)))
	})
})
