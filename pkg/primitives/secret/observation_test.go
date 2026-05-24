package secret

import (
	"testing"

	"github.com/sourcehawk/operator-component-framework/pkg/component/concepts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// The framework discovers read-only data-extraction support by type-asserting
// the registered Resource to concepts.ObservationRecorder. Because the wrapper
// holds its base resource behind a named (non-embedded) field, the
// RecordObservation method must be forwarded explicitly. This compile-time
// check guards against silent regressions of issue #118.
var _ concepts.ObservationRecorder = (*Resource)(nil)

// TestReadOnlyExtractor_ObservesClusterState reproduces the user-reported
// scenario from issue #115 / #118 end-to-end against a real *Resource: build
// the Resource via the public builder, fetch it through a fake client, hand
// the fetched object to the framework's RecordObservation hook, run
// ExtractData, and assert that the registered extractor sees the live cluster
// data rather than the inert base used to construct the resource.
//
// The framework-side wiring is covered by pkg/component/read_test.go using
// mock resources; this test exercises the same chain through a real primitive,
// so a regression in either the wrapper's forwarding or the BaseResource
// implementation is caught here.
func TestReadOnlyExtractor_ObservesClusterState(t *testing.T) {
	ctx := t.Context()

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	clusterSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "creds",
			Namespace: "ns",
		},
		Data: map[string][]byte{"token": []byte("from-cluster")},
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(clusterSecret).
		Build()

	base := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "creds",
			Namespace: "ns",
		},
	}

	var captured []byte
	res, err := NewBuilder(base).
		WithDataExtractor(func(s corev1.Secret) error {
			captured = s.Data["token"]
			return nil
		}).
		Build()
	require.NoError(t, err)

	// Simulate the framework's read flow: deep-copy the desired base, fetch
	// the cluster state into it, record the observation, run extraction.
	fetched, err := res.Object()
	require.NoError(t, err)
	require.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(fetched), fetched))

	require.NoError(t, res.RecordObservation(fetched))
	require.NoError(t, res.ExtractData())

	assert.Equal(t, []byte("from-cluster"), captured,
		"the extractor must see the cluster's Secret data, not the empty base")
}
