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
