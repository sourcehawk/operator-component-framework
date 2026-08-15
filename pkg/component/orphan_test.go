package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func ownerRef(uid types.UID, name string) metav1.OwnerReference {
	controller := true
	return metav1.OwnerReference{APIVersion: "example.io/v1", Kind: "MockOperatorCRD", Name: name, UID: uid, Controller: &controller}
}

func TestOrphanResources(t *testing.T) {
	const ns = "test-namespace"
	newOwner := func() *MockOperatorCRD {
		return &MockOperatorCRD{ObjectMeta: metav1.ObjectMeta{Name: "test-owner", Namespace: ns, UID: "owner-uid"}}
	}
	seed := func(t *testing.T, owner *MockOperatorCRD, refs ...metav1.OwnerReference) (client.Client, ReconcileContext, *MockResource) {
		scheme := setupScheme()
		fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: ns, OwnerReferences: refs}}
		require.NoError(t, fc.Create(t.Context(), cm.DeepCopy()))
		res := &MockResource{}
		res.On("Object").Return(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: ns}}, nil)
		res.On("Identity").Return("v1/ConfigMap/test-cm")
		return fc, setupReconcileContext(scheme, owner, fc), res
	}
	getLive := func(t *testing.T, fc client.Client) *corev1.ConfigMap {
		live := &corev1.ConfigMap{}
		require.NoError(t, fc.Get(t.Context(), client.ObjectKey{Name: "test-cm", Namespace: ns}, live))
		return live
	}

	t.Run("removes the owner controller reference and leaves the object", func(t *testing.T) {
		fc, rc, res := seed(t, newOwner(), ownerRef("owner-uid", "test-owner"))
		require.NoError(t, orphanResources(t.Context(), rc, []Resource{res}))
		assert.Empty(t, getLive(t, fc).GetOwnerReferences())
	})
	t.Run("idempotent when owner reference already absent", func(t *testing.T) {
		fc, rc, res := seed(t, newOwner())
		require.NoError(t, orphanResources(t.Context(), rc, []Resource{res}))
		require.NoError(t, fc.Get(t.Context(), client.ObjectKey{Name: "test-cm", Namespace: ns}, &corev1.ConfigMap{}))
	})
	t.Run("not found is a no-op", func(t *testing.T) {
		owner := newOwner()
		scheme := setupScheme()
		fc := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owner).Build()
		res := &MockResource{}
		res.On("Object").Return(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "missing", Namespace: ns}}, nil)
		res.On("Identity").Return("v1/ConfigMap/missing")
		require.NoError(t, orphanResources(t.Context(), setupReconcileContext(scheme, owner, fc), []Resource{res}))
	})
	t.Run("records an orphan event carrying the released object", func(t *testing.T) {
		owner := newOwner()
		_, rc, res := seed(t, owner, ownerRef("owner-uid", "test-owner"))
		require.NoError(t, orphanResources(t.Context(), rc, []Resource{res}))

		recorder, ok := rc.EventRecorder.(*spyRecorder)
		require.True(t, ok)

		orphaned := recorder.recordedWithReason("ResourceOrphaned")
		require.Len(t, orphaned, 1)
		assert.Equal(t, owner, orphaned[0].regarding, "event is recorded on the owner")
		assert.Equal(t, "test-cm", orphaned[0].related.(client.Object).GetName(),
			"released resource is attached as the related object")
		assert.Equal(t, corev1.EventTypeNormal, orphaned[0].eventType)
		assert.Equal(t, "Orphan", orphaned[0].action)
		assert.Equal(t, "Resource v1/ConfigMap/test-cm orphaned: owner reference removed", orphaned[0].note)
	})
	t.Run("records no event when the owner reference was already absent", func(t *testing.T) {
		_, rc, res := seed(t, newOwner())
		require.NoError(t, orphanResources(t.Context(), rc, []Resource{res}))

		recorder, ok := rc.EventRecorder.(*spyRecorder)
		require.True(t, ok)
		assert.Empty(t, recorder.recorded())
	})
	t.Run("removes only this owner's reference", func(t *testing.T) {
		fc, rc, res := seed(t, newOwner(), ownerRef("owner-uid", "test-owner"), ownerRef("other-uid", "other-owner"))
		require.NoError(t, orphanResources(t.Context(), rc, []Resource{res}))
		live := getLive(t, fc)
		require.Len(t, live.GetOwnerReferences(), 1)
		assert.Equal(t, types.UID("other-uid"), live.GetOwnerReferences()[0].UID)
	})
}
