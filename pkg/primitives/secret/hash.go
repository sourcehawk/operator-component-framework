package secret

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// DataHash computes a stable SHA-256 hash of the .data field of the given Secret.
//
// The hash is derived from the canonical JSON encoding of .data with map keys
// sorted alphabetically, so it is deterministic regardless of insertion order.
// The returned string is the lowercase hex encoding of the 256-bit digest.
//
// Only .data is hashed. The .stringData field is write-only in the Kubernetes API
// and is absent from objects returned by the API server; it is intentionally
// excluded so that DataHash is consistent whether called on a desired object or
// a cluster-read object.
//
// A common use case is to annotate a Deployment's pod template with this hash
// so that a change in Secret content triggers a rolling restart:
//
//	hash, err := secret.DataHash(s)
//	if err != nil {
//	    return err
//	}
//	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
//	    e.EnsureAnnotation("checksum/secret", hash)
//	    return nil
//	})
func DataHash(s corev1.Secret) (string, error) {
	// Normalize nil to empty so that a Secret with no .data hashes identically
	// to one with an empty map — both represent "no entries".
	data := s.Data
	if data == nil {
		data = map[string][]byte{}
	}

	encoded, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("secret %s/%s: failed to marshal data for hashing: %w",
			s.Namespace, s.Name, err)
	}

	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// DesiredHash computes the SHA-256 hash of the Secret as it will be written to
// the cluster, based on the base object and all registered mutations.
//
// The hash covers only operator-controlled fields (.data after applying the
// baseline and mutations). Fields preserved by flavors from the live cluster
// state (e.g. PreserveExternalEntries) are intentionally excluded — only
// changes to operator-owned content will change the hash.
//
// This enables a single-pass checksum pattern: compute the hash before
// reconciliation and pass it directly to the deployment resource factory,
// avoiding the need for a second reconcile cycle.
//
//	secretResource, err := secret.NewBuilder(base).WithMutation(...).Build()
//	hash, err := secretResource.DesiredHash()
//	deployResource, err := deployment.NewBuilder(base).
//	    WithMutation(ChecksumMutation(version, hash)).
//	    Build()
func (r *Resource) DesiredHash() (string, error) {
	obj, err := r.base.PreviewObject()
	if err != nil {
		return "", fmt.Errorf("secret %s: failed to compute desired hash: %w", r.Identity(), err)
	}

	return DataHash(*obj)
}
