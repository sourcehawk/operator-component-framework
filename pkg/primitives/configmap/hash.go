package configmap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// DataHash computes a stable SHA-256 hash of the .data and .binaryData fields
// of the given ConfigMap.
//
// The hash is derived from the canonical JSON encoding of both fields with map
// keys sorted alphabetically, so it is deterministic regardless of insertion
// order. The returned string is the lowercase hex encoding of the 256-bit digest.
//
// A common use case is to annotate a Deployment's pod template with this hash
// so that a change in ConfigMap content triggers a rolling restart:
//
//	hash, err := configmap.DataHash(cm)
//	if err != nil {
//	    return err
//	}
//	m.EditPodTemplateMetadata(func(e *editors.ObjectMetaEditor) error {
//	    e.EnsureAnnotation("checksum/config", hash)
//	    return nil
//	})
func DataHash(cm corev1.ConfigMap) (string, error) {
	type payload struct {
		Data       map[string]string `json:"data,omitempty"`
		BinaryData map[string][]byte `json:"binaryData,omitempty"`
	}

	p := payload{Data: cm.Data, BinaryData: cm.BinaryData}

	encoded, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("configmap %s/%s: failed to marshal data for hashing: %w",
			cm.Namespace, cm.Name, err)
	}

	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// DesiredHash computes the SHA-256 hash of the ConfigMap as it will be written to
// the cluster, based on the base object and all registered mutations.
//
// The hash covers only operator-controlled fields (.data and .binaryData after
// applying the baseline and mutations). Fields preserved by flavors from the live
// cluster state (e.g. PreserveExternalEntries) are intentionally excluded — only
// changes to operator-owned content will change the hash.
//
// This enables a single-pass checksum pattern: compute the hash before
// reconciliation and pass it directly to the deployment resource factory, avoiding
// the need for a second reconcile cycle.
//
//	cmResource, err := configmap.NewBuilder(base).WithMutation(...).Build()
//	hash, err := cmResource.DesiredHash()
//	deployResource, err := deployment.NewBuilder(base).
//	    WithMutation(ChecksumMutation(version, hash)).
//	    Build()
func (r *Resource) DesiredHash() (string, error) {
	obj, err := r.base.PreviewObject()
	if err != nil {
		return "", fmt.Errorf("configmap %s: failed to compute desired hash: %w", r.Identity(), err)
	}

	return DataHash(*obj)
}
