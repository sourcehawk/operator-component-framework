package generic

import (
	"errors"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newDataTestBuilder() *StaticBuilder[*corev1.ConfigMap, *mockMutator] {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
		Data:       map[string]string{"db-host": "postgres.default.svc"},
	}
	return NewStaticBuilder[*corev1.ConfigMap, *mockMutator](
		cm,
		func(c *corev1.ConfigMap) string { return "v1/ConfigMap/" + c.Namespace + "/" + c.Name },
		func(*corev1.ConfigMap) *mockMutator { return &mockMutator{} },
	)
}

func TestExtractIntoSetsCellOnExtractData(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	b := newDataTestBuilder()
	ExtractInto(&b.BaseBuilder, cell, func(cm *corev1.ConfigMap) (string, error) {
		return cm.Data["db-host"], nil
	})

	res, err := b.Build()
	require.NoError(t, err)
	assert.False(t, cell.IsSet())

	require.NoError(t, res.ExtractData())

	v, ok := cell.Get()
	assert.True(t, ok)
	assert.Equal(t, "postgres.default.svc", v)
}

func TestExtractIntoErrorLeavesCellUnsetAndNamesCell(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	b := newDataTestBuilder()
	ExtractInto(&b.BaseBuilder, cell, func(*corev1.ConfigMap) (string, error) {
		return "", errors.New("boom")
	})

	res, err := b.Build()
	require.NoError(t, err)

	extractErr := res.ExtractData()
	require.Error(t, extractErr)
	assert.Contains(t, extractErr.Error(), `"db-host"`)
	assert.Contains(t, extractErr.Error(), "boom")
	assert.False(t, cell.IsSet())
}

func TestProducedDataOrderAndDedupe(t *testing.T) {
	host := concepts.NewData[string]("db-host")
	port := concepts.NewData[string]("db-port")
	b := newDataTestBuilder()
	ExtractInto(&b.BaseBuilder, host, func(cm *corev1.ConfigMap) (string, error) { return cm.Data["db-host"], nil })
	ExtractInto(&b.BaseBuilder, port, func(cm *corev1.ConfigMap) (string, error) { return cm.Data["db-port"], nil })
	ExtractInto(&b.BaseBuilder, host, func(cm *corev1.ConfigMap) (string, error) { return cm.Data["db-host"], nil })

	res, err := b.Build()
	require.NoError(t, err)

	produced := res.ProducedData()
	require.Len(t, produced, 2)
	assert.Same(t, host, produced[0].(*concepts.Data[string]))
	assert.Same(t, port, produced[1].(*concepts.Data[string]))
}

func TestExtractIntoLastProducerWins(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	b := newDataTestBuilder()
	ExtractInto(&b.BaseBuilder, cell, func(*corev1.ConfigMap) (string, error) { return "first", nil })
	ExtractInto(&b.BaseBuilder, cell, func(*corev1.ConfigMap) (string, error) { return "second", nil })

	res, err := b.Build()
	require.NoError(t, err)
	require.NoError(t, res.ExtractData())

	v, ok := cell.Get()
	assert.True(t, ok)
	assert.Equal(t, "second", v)
}

func TestExtractIntoNilCellRejectedAtBuild(t *testing.T) {
	b := newDataTestBuilder()
	ExtractInto[*corev1.ConfigMap, *mockMutator, string](&b.BaseBuilder, nil, func(*corev1.ConfigMap) (string, error) {
		return "", nil
	})

	_, err := b.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-nil cell")
}

func TestExtractIntoNilFuncRejectedAtBuild(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	b := newDataTestBuilder()
	ExtractInto[*corev1.ConfigMap, *mockMutator, string](&b.BaseBuilder, cell, nil)

	_, err := b.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-nil extraction function")
}

func TestWrapExtraction(t *testing.T) {
	fn := WrapExtraction(func(cm corev1.ConfigMap) (string, error) { return cm.Data["k"], nil })
	v, err := fn(&corev1.ConfigMap{Data: map[string]string{"k": "v"}})
	require.NoError(t, err)
	assert.Equal(t, "v", v)

	assert.Nil(t, WrapExtraction[corev1.ConfigMap, string](nil))
}

func TestWithDataGuardBlocksUntilSet(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	b := newDataTestBuilder()
	b.WithDataGuard(cell)

	res, err := b.Build()
	require.NoError(t, err)

	status, err := res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusBlocked, status.Status)
	assert.Equal(t, `waiting for data "db-host"`, status.Reason)

	cell.Set("postgres.default.svc")
	status, err = res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusUnblocked, status.Status)
}

func TestWithDataGuardListsAllMissingCells(t *testing.T) {
	host := concepts.NewData[string]("db-host")
	port := concepts.NewData[string]("db-port")
	b := newDataTestBuilder()
	b.WithDataGuard(host, port)

	res, err := b.Build()
	require.NoError(t, err)

	status, err := res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusBlocked, status.Status)
	assert.Equal(t, `waiting for data "db-host", "db-port"`, status.Reason)
}

func TestWithDataGuardRunsBeforeCustomGuard(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	b := newDataTestBuilder()
	b.WithDataGuard(cell)
	customCalled := false
	b.WithGuard(func(*corev1.ConfigMap) (concepts.GuardStatusWithReason, error) {
		customCalled = true
		return concepts.GuardStatusWithReason{Status: concepts.GuardStatusUnblocked}, nil
	})

	res, err := b.Build()
	require.NoError(t, err)

	status, err := res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusBlocked, status.Status)
	assert.False(t, customCalled)

	cell.Set("x")
	status, err = res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusUnblocked, status.Status)
	assert.True(t, customCalled)
}

func TestWithOptionalDataNeverBlocks(t *testing.T) {
	cell := concepts.NewData[string]("db-host")
	b := newDataTestBuilder()
	b.WithOptionalData(cell)

	res, err := b.Build()
	require.NoError(t, err)

	status, err := res.GuardStatus()
	require.NoError(t, err)
	assert.Equal(t, concepts.GuardStatusUnblocked, status.Status)
}

func TestConsumedDataDeclarationOrderAndModes(t *testing.T) {
	host := concepts.NewData[string]("db-host")
	port := concepts.NewData[string]("db-port")
	b := newDataTestBuilder()
	b.WithDataGuard(host)
	b.WithOptionalData(port)

	res, err := b.Build()
	require.NoError(t, err)

	consumed := res.ConsumedData()
	require.Len(t, consumed, 2)
	assert.Same(t, host, consumed[0].Cell.(*concepts.Data[string]))
	assert.False(t, consumed[0].Optional)
	assert.Same(t, port, consumed[1].Cell.(*concepts.Data[string]))
	assert.True(t, consumed[1].Optional)
}

func TestDataReadNilCellRejectedAtBuild(t *testing.T) {
	b := newDataTestBuilder()
	b.WithDataGuard(nil)

	_, err := b.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-nil cell")
}
