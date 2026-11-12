package component

import (
	"context"
	"maps"
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/structured-merge-diff/v6/fieldpath"
	"sigs.k8s.io/structured-merge-diff/v6/value"
)

// concurrentWriteEnv is a reconcile context whose client returns managedFields,
// as the API server does, and runs between once right after the next read of a
// ConfigMap. That is the window between the read of the component and its apply.
type concurrentWriteEnv struct {
	rec     ReconcileContext
	client  client.Client
	between func(ctx context.Context, c client.WithWatch) error
}

func newConcurrentWriteEnv(t *testing.T, stripReadManagedFields bool) *concurrentWriteEnv {
	t.Helper()
	env := &concurrentWriteEnv{}
	scheme := setupScheme()
	owner := &MockOperatorCRD{
		ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: "test-namespace", UID: "owner-uid"},
	}
	env.client = fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(owner).
		WithStatusSubresource(owner).
		WithReturnManagedFields().
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(
				ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
			) error {
				if err := c.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				if _, ok := obj.(*corev1.ConfigMap); !ok {
					return nil
				}
				if stripReadManagedFields {
					obj.SetManagedFields(nil)
				}
				if env.between == nil {
					return nil
				}
				write := env.between
				env.between = nil
				return write(ctx, c)
			},
		}).
		Build()
	env.rec = setupReconcileContext(scheme, owner, env.client)
	return env
}

func (e *concurrentWriteEnv) apply(t *testing.T, res Resource) {
	t.Helper()
	_, err := applyResources(t.Context(), e.rec, []reconcileEntry{{Resource: res}}, "test-component", createTestRESTMapper())
	require.NoError(t, err)
	require.Nil(t, e.between, "the concurrent write must have run")
}

func (e *concurrentWriteEnv) updatedEvents() []recordedEvent {
	return e.rec.EventRecorder.(*spyRecorder).recordedWithReason("UpdatedConfigMap")
}

const concurrentWriteConfigMap = "concurrent-write-cm"

// writeByOtherManager returns a write that another controller makes to the
// ConfigMap with an Update, under its own field manager.
func writeByOtherManager(edit func(*corev1.ConfigMap)) func(context.Context, client.WithWatch) error {
	return func(ctx context.Context, c client.WithWatch) error {
		cm := &corev1.ConfigMap{}
		key := client.ObjectKey{Name: concurrentWriteConfigMap, Namespace: "test-namespace"}
		if err := c.Get(ctx, key, cm); err != nil {
			return err
		}
		edit(cm)
		return c.Update(ctx, cm, client.FieldOwner("other-controller"))
	}
}

func buildConcurrentWriteConfigMap(data map[string]string) func() *corev1.ConfigMap {
	return func() *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: concurrentWriteConfigMap, Namespace: "test-namespace"},
			Data:       maps.Clone(data),
		}
	}
}

func TestApplyResource_ClassifiesOnOwnedFields(t *testing.T) {
	otherWrites := map[string]func(*corev1.ConfigMap){
		"a finalizer": func(cm *corev1.ConfigMap) {
			cm.Finalizers = append(cm.Finalizers, "other.example.com/cleanup")
		},
		"a label": func(cm *corev1.ConfigMap) {
			cm.Labels = map[string]string{"other.example.com/seen": "true"}
		},
		"an annotation": func(cm *corev1.ConfigMap) {
			cm.Annotations = map[string]string{"other.example.com/seen": "true"}
		},
	}
	for name, edit := range otherWrites {
		t.Run("reports None when another manager adds "+name+" between the read and the apply", func(t *testing.T) {
			env := newConcurrentWriteEnv(t, false)
			res := &operationRecordingResource{build: buildConcurrentWriteConfigMap(map[string]string{"foo": "bar"})}
			env.apply(t, res)

			env.between = writeByOtherManager(edit)
			env.apply(t, res)

			assert.Equal(t, []concepts.ConvergingOperation{
				concepts.ConvergingOperationCreated,
				concepts.ConvergingOperationNone,
			}, res.operations)
			assert.Empty(t, env.updatedEvents())
		})
	}

	t.Run("reports Updated once when an owned field changes during a concurrent write", func(t *testing.T) {
		env := newConcurrentWriteEnv(t, false)
		res := &operationRecordingResource{build: buildConcurrentWriteConfigMap(map[string]string{"foo": "bar"})}
		env.apply(t, res)

		res.build = buildConcurrentWriteConfigMap(map[string]string{"foo": "baz"})
		env.between = writeByOtherManager(otherWrites["a finalizer"])
		env.apply(t, res)
		env.apply(t, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationUpdated,
			concepts.ConvergingOperationNone,
		}, res.operations)
		assert.Len(t, env.updatedEvents(), 1)
	})

	t.Run("reports Updated once when the apply removes an owned field", func(t *testing.T) {
		env := newConcurrentWriteEnv(t, false)
		res := &operationRecordingResource{
			build: buildConcurrentWriteConfigMap(map[string]string{"foo": "bar", "removed": "soon"}),
		}
		env.apply(t, res)

		res.build = buildConcurrentWriteConfigMap(map[string]string{"foo": "bar"})
		env.apply(t, res)
		env.apply(t, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationUpdated,
			concepts.ConvergingOperationNone,
		}, res.operations)
		assert.Len(t, env.updatedEvents(), 1)

		live := &corev1.ConfigMap{}
		require.NoError(t, env.client.Get(
			t.Context(), client.ObjectKey{Name: concurrentWriteConfigMap, Namespace: "test-namespace"}, live,
		))
		assert.Equal(t, map[string]string{"foo": "bar"}, live.Data)
	})

	t.Run("reports Updated when the apply removes an owned field and the read carries no managedFields", func(t *testing.T) {
		env := newConcurrentWriteEnv(t, true)
		res := &operationRecordingResource{
			build: buildConcurrentWriteConfigMap(map[string]string{"foo": "bar", "removed": "soon"}),
		}
		env.apply(t, res)

		res.build = buildConcurrentWriteConfigMap(map[string]string{"foo": "bar"})
		env.apply(t, res)
		env.apply(t, res)

		assert.Equal(t, []concepts.ConvergingOperation{
			concepts.ConvergingOperationCreated,
			concepts.ConvergingOperationUpdated,
			concepts.ConvergingOperationNone,
		}, res.operations)
	})
}

func TestLookupFieldPath(t *testing.T) {
	content := map[string]any{
		"spec": map[string]any{
			"replicas": int64(2),
			"containers": []any{
				map[string]any{
					"name":  "app",
					"image": "app:1",
					"ports": []any{
						map[string]any{"containerPort": int64(8080), "protocol": "TCP", "name": "http"},
						map[string]any{"containerPort": int64(8080), "protocol": "UDP", "name": "quic"},
					},
				},
			},
			"args":       []any{"--verbose", "--port=8080"},
			"finalizers": []any{"a", "b"},
		},
	}

	tests := []struct {
		name      string
		path      fieldpath.Path
		wantValue any
		wantFound bool
	}{
		{
			name:      "field of a map",
			path:      fieldpath.MakePathOrDie("spec", "replicas"),
			wantValue: int64(2),
			wantFound: true,
		},
		{
			name:      "absent field",
			path:      fieldpath.MakePathOrDie("spec", "paused"),
			wantFound: false,
		},
		{
			name:      "item of an associative list by its key",
			path:      fieldpath.MakePathOrDie("spec", "containers", fieldpath.KeyByFields("name", "app"), "image"),
			wantValue: "app:1",
			wantFound: true,
		},
		{
			name: "item of an associative list by a key of several fields",
			path: fieldpath.MakePathOrDie(
				"spec", "containers", fieldpath.KeyByFields("name", "app"),
				"ports", fieldpath.KeyByFields("containerPort", 8080, "protocol", "UDP"), "name",
			),
			wantValue: "quic",
			wantFound: true,
		},
		{
			name:      "absent item of an associative list",
			path:      fieldpath.MakePathOrDie("spec", "containers", fieldpath.KeyByFields("name", "sidecar")),
			wantFound: false,
		},
		{
			name:      "item of a set by its value",
			path:      fieldpath.MakePathOrDie("spec", "finalizers", value.NewValueInterface("b")),
			wantValue: "b",
			wantFound: true,
		},
		{
			name:      "absent item of a set",
			path:      fieldpath.MakePathOrDie("spec", "finalizers", value.NewValueInterface("c")),
			wantFound: false,
		},
		{
			name:      "item of an atomic list by its index",
			path:      fieldpath.MakePathOrDie("spec", "args", 1),
			wantValue: "--port=8080",
			wantFound: true,
		},
		{
			name:      "index past the end of a list",
			path:      fieldpath.MakePathOrDie("spec", "args", 2),
			wantFound: false,
		},
		{
			name:      "field name on a value that is not a map",
			path:      fieldpath.MakePathOrDie("spec", "replicas", "value"),
			wantFound: false,
		},
		{
			name:      "key on a value that is not a list",
			path:      fieldpath.MakePathOrDie("spec", fieldpath.KeyByFields("name", "app")),
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := lookupFieldPath(content, tt.path)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantValue, got)
		})
	}
}

func TestOwnedListOrderChanged(t *testing.T) {
	containers := func(names ...string) map[string]any {
		items := make([]any, 0, len(names))
		for _, name := range names {
			items = append(items, map[string]any{"name": name})
		}
		return map[string]any{"spec": map[string]any{"containers": items}}
	}
	owned := fieldpath.NewSet(
		fieldpath.MakePathOrDie("spec", "containers", fieldpath.KeyByFields("name", "first")),
		fieldpath.MakePathOrDie("spec", "containers", fieldpath.KeyByFields("name", "second")),
	)

	tests := []struct {
		name   string
		before map[string]any
		after  map[string]any
		want   bool
	}{
		{
			name:   "same order",
			before: containers("first", "second"),
			after:  containers("first", "second"),
			want:   false,
		},
		{
			name:   "owned items swapped",
			before: containers("first", "second"),
			after:  containers("second", "first"),
			want:   true,
		},
		{
			name:   "item of another manager inserted between owned items",
			before: containers("first", "second"),
			after:  containers("first", "sidecar", "second"),
			want:   false,
		},
		{
			name:   "item of another manager moved ahead of owned items",
			before: containers("first", "sidecar", "second"),
			after:  containers("sidecar", "first", "second"),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ownedListOrderChanged(owned, nil, tt.before, tt.after))
		})
	}
}

func TestAppliedObjectChanged_ComparesWholeObjectWhenEntryVersionsDiffer(t *testing.T) {
	const manager = "test-manager"
	entry := func(apiVersion, fields string) metav1.ManagedFieldsEntry {
		raw := &metav1.FieldsV1{}
		raw.SetRawString(fields)
		return metav1.ManagedFieldsEntry{
			Manager:    manager,
			Operation:  metav1.ManagedFieldsOperationApply,
			APIVersion: apiVersion,
			FieldsType: "FieldsV1",
			FieldsV1:   raw,
		}
	}

	// The entry before the apply was recorded at an older version, where the
	// field had another name. The apply removes the field.
	before := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:          "converted",
			ManagedFields: []metav1.ManagedFieldsEntry{entry("v1beta1", `{"f:data":{"f:oldName":{}}}`)},
		},
		Data: map[string]string{"newName": "value"},
	}
	after := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:          "converted",
			ManagedFields: []metav1.ManagedFieldsEntry{entry("v1", `{"f:metadata":{"f:name":{}}}`)},
		},
	}

	changed, err := appliedObjectChanged(before, after, manager)
	require.NoError(t, err)
	assert.True(t, changed)
}
