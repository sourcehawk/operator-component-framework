package pv

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestDefaultFieldApplicator_Create(t *testing.T) {
	current := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
	}
	volumeMode := corev1.PersistentVolumeFilesystem
	desired := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pv",
			Labels: map[string]string{"app": "myapp"},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Server: "nfs.example.com",
					Path:   "/exports/data",
				},
			},
			VolumeMode:                    &volumeMode,
			StorageClassName:              "fast-ssd",
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// On create (empty ResourceVersion), all fields should be applied.
	assert.Equal(t, "myapp", current.Labels["app"])
	expectedCapacity := resource.MustParse("10Gi")
	actualCapacity := current.Spec.Capacity[corev1.ResourceStorage]
	assert.True(t, expectedCapacity.Equal(actualCapacity), "expected storage capacity %s, got %s", expectedCapacity.String(), actualCapacity.String())
	assert.Equal(t, "nfs.example.com", current.Spec.NFS.Server)
	assert.Equal(t, &volumeMode, current.Spec.VolumeMode)
	assert.Equal(t, "fast-ssd", current.Spec.StorageClassName)
	assert.Equal(t, corev1.PersistentVolumeReclaimRetain, current.Spec.PersistentVolumeReclaimPolicy)
}

func TestDefaultFieldApplicator_Update_PreservesImmutableFields(t *testing.T) {
	existingVolumeMode := corev1.PersistentVolumeBlock
	claimRef := &corev1.ObjectReference{
		Namespace: "default",
		Name:      "my-pvc",
	}

	current := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pv",
			ResourceVersion: "12345",
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Server: "nfs.example.com",
					Path:   "/exports/data",
				},
			},
			VolumeMode: &existingVolumeMode,
			ClaimRef:   claimRef,
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("10Gi"),
			},
			StorageClassName:              "old-class",
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
		},
	}

	desiredVolumeMode := corev1.PersistentVolumeFilesystem
	desired := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pv",
			Labels: map[string]string{"updated": "true"},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/mnt/data",
				},
			},
			VolumeMode: &desiredVolumeMode,
			ClaimRef: &corev1.ObjectReference{
				Namespace: "other",
				Name:      "other-pvc",
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("20Gi"),
			},
			StorageClassName:              "new-class",
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Immutable fields should be preserved from the existing object.
	assert.Equal(t, "nfs.example.com", current.Spec.NFS.Server,
		"PersistentVolumeSource should be preserved on update")
	assert.Nil(t, current.Spec.HostPath,
		"desired volume source should not overwrite existing")
	assert.Equal(t, &existingVolumeMode, current.Spec.VolumeMode,
		"VolumeMode should be preserved on update")
	assert.Equal(t, "my-pvc", current.Spec.ClaimRef.Name,
		"ClaimRef should be preserved on update")

	// Mutable fields should be updated from desired.
	assert.Equal(t, "true", current.Labels["updated"],
		"labels should be updated from desired")
	expectedUpdatedCapacity := resource.MustParse("20Gi")
	actualUpdatedCapacity := current.Spec.Capacity[corev1.ResourceStorage]
	assert.True(t, expectedUpdatedCapacity.Equal(actualUpdatedCapacity),
		"capacity should be updated from desired: expected %s, got %s", expectedUpdatedCapacity.String(), actualUpdatedCapacity.String())
	assert.Equal(t, "new-class", current.Spec.StorageClassName,
		"storage class should be updated from desired")
	assert.Equal(t, corev1.PersistentVolumeReclaimRetain, current.Spec.PersistentVolumeReclaimPolicy,
		"reclaim policy should be updated from desired")
}

func TestDefaultFieldApplicator_Update_NilImmutableFields(t *testing.T) {
	// When existing object has nil immutable fields, they should stay nil
	// (not be overwritten by desired values).
	current := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pv",
			ResourceVersion: "1",
		},
		Spec: corev1.PersistentVolumeSpec{},
	}

	volumeMode := corev1.PersistentVolumeFilesystem
	desired := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Server: "nfs.example.com",
					Path:   "/data",
				},
			},
			VolumeMode: &volumeMode,
			ClaimRef: &corev1.ObjectReference{
				Name: "some-pvc",
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Immutable fields from current (all zero/nil) should be restored.
	assert.Nil(t, current.Spec.NFS,
		"nil volume source should be preserved")
	assert.Nil(t, current.Spec.VolumeMode,
		"nil volume mode should be preserved")
	assert.Nil(t, current.Spec.ClaimRef,
		"nil claim ref should be preserved")
}

func TestDefaultFieldApplicator_DoesNotMutateDesired(t *testing.T) {
	current := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pv",
			ResourceVersion: "1",
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Server: "existing.example.com",
					Path:   "/old",
				},
			},
		},
	}

	desired := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName: "desired-class",
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Server: "desired.example.com",
					Path:   "/new",
				},
			},
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired object must not be mutated.
	assert.Equal(t, "desired.example.com", desired.Spec.NFS.Server)
	assert.Equal(t, "desired-class", desired.Spec.StorageClassName)
}

func TestDefaultFieldApplicator_PreservesStatus(t *testing.T) {
	current := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pv",
			ResourceVersion: "12345",
		},
		Status: corev1.PersistentVolumeStatus{
			Phase:   corev1.VolumeBound,
			Message: "bound to claim default/data",
			Reason:  "Bound",
		},
	}
	desired := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName: "fast-ssd",
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec is applied
	assert.Equal(t, "fast-ssd", current.Spec.StorageClassName)

	// Status from the live object is preserved
	assert.Equal(t, corev1.VolumeBound, current.Status.Phase)
	assert.Equal(t, "bound to claim default/data", current.Status.Message)
	assert.Equal(t, "Bound", current.Status.Reason)
}

func TestDefaultFieldApplicator_PreservesServerManagedFields(t *testing.T) {
	existingVolumeMode := corev1.PersistentVolumeBlock
	current := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-pv",
			ResourceVersion: "12345",
			UID:             types.UID("abc-def"),
			Generation:      3,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "v1", Kind: "Pod", Name: "other-owner", UID: "other-uid"},
			},
			Finalizers: []string{"finalizer.example.com"},
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				NFS: &corev1.NFSVolumeSource{
					Server: "nfs.example.com",
					Path:   "/exports/data",
				},
			},
			VolumeMode: &existingVolumeMode,
		},
	}
	desired := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pv",
			Labels: map[string]string{"app": "test"},
		},
		Spec: corev1.PersistentVolumeSpec{
			StorageClassName:              "fast-ssd",
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
		},
	}

	err := DefaultFieldApplicator(current, desired)
	require.NoError(t, err)

	// Desired spec and labels are applied
	assert.Equal(t, "fast-ssd", current.Spec.StorageClassName)
	assert.Equal(t, "test", current.Labels["app"])

	// Server-managed fields are preserved
	assert.Equal(t, "12345", current.ResourceVersion)
	assert.Equal(t, "abc-def", string(current.UID))
	assert.Equal(t, int64(3), current.Generation)

	// Shared-controller fields are preserved
	assert.Len(t, current.OwnerReferences, 1)
	assert.Equal(t, "other-owner", current.OwnerReferences[0].Name)
	assert.Equal(t, []string{"finalizer.example.com"}, current.Finalizers)

	// PV-specific immutable fields are preserved
	assert.Equal(t, "nfs.example.com", current.Spec.NFS.Server)
	assert.Equal(t, &existingVolumeMode, current.Spec.VolumeMode)
}
