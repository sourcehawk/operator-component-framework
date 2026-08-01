package concepts

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataStartsUnset(t *testing.T) {
	d := NewData[string]("db-host")

	assert.Equal(t, "db-host", d.Name())
	assert.False(t, d.IsSet())

	v, ok := d.Get()
	assert.False(t, ok)
	assert.Empty(t, v)
}

func TestDataRequireWhenUnset(t *testing.T) {
	d := NewData[string]("db-host")

	v, err := d.Require()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrDataNotExtracted))
	assert.Contains(t, err.Error(), `"db-host"`)
	assert.Empty(t, v)
}

func TestDataSetMarksPresent(t *testing.T) {
	d := NewData[string]("db-host")
	d.Set("postgres.default.svc")

	assert.True(t, d.IsSet())

	v, ok := d.Get()
	assert.True(t, ok)
	assert.Equal(t, "postgres.default.svc", v)

	rv, err := d.Require()
	require.NoError(t, err)
	assert.Equal(t, "postgres.default.svc", rv)
}

func TestDataSetZeroValueIsPresent(t *testing.T) {
	d := NewData[string]("maybe-empty")
	d.Set("")

	assert.True(t, d.IsSet())
	v, ok := d.Get()
	assert.True(t, ok)
	assert.Empty(t, v)
}

func TestDataClearResetsValueAndPresence(t *testing.T) {
	d := NewData[int]("replicas")
	d.Set(3)
	d.Clear()

	assert.False(t, d.IsSet())
	v, ok := d.Get()
	assert.False(t, ok)
	assert.Zero(t, v)
}

func TestDataSatisfiesDataCell(t *testing.T) {
	var cell DataCell = NewData[string]("x")
	assert.Equal(t, "x", cell.Name())
}
