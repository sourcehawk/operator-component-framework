package component

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// fakeDataResource is a minimal Resource with declared data produced and
// consumed. Build-time validation never touches Object or Mutate.
type fakeDataResource struct {
	identity string
	produced []concepts.DataCell
	consumed []concepts.DataConsumption
}

func (f *fakeDataResource) Identity() string                         { return f.identity }
func (f *fakeDataResource) Object() (client.Object, error)           { return nil, nil }
func (f *fakeDataResource) Mutate(client.Object) error               { return nil }
func (f *fakeDataResource) ProducedData() []concepts.DataCell        { return f.produced }
func (f *fakeDataResource) ConsumedData() []concepts.DataConsumption { return f.consumed }

func newDataComponentBuilder() *Builder {
	return NewComponentBuilder().WithName("data-test").WithConditionType("DataReady")
}

func TestBuildRejectsGuardedReadWithNoProducer(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	consumer := &fakeDataResource{
		identity: "v1/Secret/default/creds",
		consumed: []concepts.DataConsumption{{Cell: cell}},
	}

	_, err := newDataComponentBuilder().WithResource(consumer).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `resource "v1/Secret/default/creds" reads data "db-host" but no earlier resource produces it`)
}

func TestBuildRejectsOptionalReadWithNoProducer(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	consumer := &fakeDataResource{
		identity: "v1/Secret/default/creds",
		consumed: []concepts.DataConsumption{{Cell: cell, Optional: true}},
	}

	_, err := newDataComponentBuilder().WithResource(consumer).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `reads data "db-host" but no earlier resource produces it`)
}

func TestBuildRejectsProducerRegisteredAfterConsumer(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	consumer := &fakeDataResource{
		identity: "v1/Secret/default/creds",
		consumed: []concepts.DataConsumption{{Cell: cell}},
	}
	producer := &fakeDataResource{
		identity: "v1/ConfigMap/default/config",
		produced: []concepts.DataCell{cell},
	}

	_, err := newDataComponentBuilder().WithResource(consumer).WithResource(producer).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no earlier resource produces it")
}

func TestBuildRejectsDistinctCellsSharingAName(t *testing.T) {
	a := concepts.NewData[string]("db-host")
	b := concepts.NewData[int]("db-host")
	producerA := &fakeDataResource{identity: "v1/ConfigMap/default/a", produced: []concepts.DataCell{a}}
	producerB := &fakeDataResource{identity: "v1/ConfigMap/default/b", produced: []concepts.DataCell{b}}

	_, err := newDataComponentBuilder().WithResource(producerA).WithResource(producerB).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"db-host"`)
	assert.Contains(t, err.Error(), "distinct")
}

func TestBuildAllowsMultipleProducers(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	first := &fakeDataResource{identity: "v1/ConfigMap/default/a", produced: []concepts.DataCell{cell}}
	second := &fakeDataResource{identity: "v1/ConfigMap/default/b", produced: []concepts.DataCell{cell}}
	consumer := &fakeDataResource{
		identity: "v1/Secret/default/creds",
		consumed: []concepts.DataConsumption{{Cell: cell}},
	}

	comp, err := newDataComponentBuilder().
		WithResource(first).WithResource(second).WithResource(consumer).Build()
	require.NoError(t, err)
	require.NotNil(t, comp)
}

func TestBuildAcceptsValidTopologyAndCollectsCells(t *testing.T) {
	host := concepts.NewData[string]("db-host")
	port := concepts.NewData[string]("db-port")
	producer := &fakeDataResource{identity: "v1/ConfigMap/default/config", produced: []concepts.DataCell{host, port}}
	consumer := &fakeDataResource{
		identity: "v1/Secret/default/creds",
		consumed: []concepts.DataConsumption{{Cell: host}, {Cell: port, Optional: true}},
	}

	comp, err := newDataComponentBuilder().WithResource(producer).WithResource(consumer).Build()
	require.NoError(t, err)
	require.Len(t, comp.dataCells, 2)
	assert.Same(t, host, comp.dataCells[0].(*concepts.Data[string]))
	assert.Same(t, port, comp.dataCells[1].(*concepts.Data[string]))
}

func TestBuildIgnoresResourcesWithoutDataDeclarations(t *testing.T) {
	plain := &fakeDataResource{identity: "v1/ConfigMap/default/plain"}

	comp, err := newDataComponentBuilder().WithResource(plain).Build()
	require.NoError(t, err)
	assert.Empty(t, comp.dataCells)
}
