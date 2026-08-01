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
