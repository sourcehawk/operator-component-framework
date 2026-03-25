package secret

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// DataHash computes a stable SHA-256 hash of the effective data content of the
// given Secret.
//
// The hash is derived from the canonical JSON encoding of the merged data map
// with keys sorted alphabetically, so it is deterministic regardless of
// insertion order. The returned string is the lowercase hex encoding of the
// 256-bit digest.
//
// Both .data and .stringData are included. To match Kubernetes API-server write
// semantics, .stringData entries are merged into a copy of .data (with
// .stringData keys taking precedence) before hashing. This ensures the hash is
// consistent whether called on a desired object (which may use .stringData) or
// a cluster-read object (where the API server has already merged .stringData
// into .data).
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
	// Build the effective data map by merging .stringData into a copy of .data,
	// mirroring the Kubernetes API server's write semantics.
	data := make(map[string][]byte, len(s.Data)+len(s.StringData))
	for k, v := range s.Data {
		data[k] = v
	}
	for k, v := range s.StringData {
		data[k] = []byte(v)
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
// The hash covers only operator-controlled data content: the effective Secret
// data after applying the baseline and mutations, with .stringData merged into
// .data (and .stringData keys taking precedence), matching DataHash semantics.
// Fields preserved by flavors from the live cluster state (e.g.
// PreserveExternalEntries) are intentionally excluded — only changes to
// operator-owned content will change the hash.
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
