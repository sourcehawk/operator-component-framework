package editors

import (
	"fmt"

	"sigs.k8s.io/yaml"
)

// ConfigMapDataEditor provides a typed API for mutating the .data and .binaryData
// fields of a Kubernetes ConfigMap.
//
// It exposes structured operations (Set, Remove, MergeYAML, SetBinary,
// RemoveBinary) as well as Raw() and RawBinary() for free-form access when
// none of the structured methods are sufficient.
type ConfigMapDataEditor struct {
	data       *map[string]string
	binaryData *map[string][]byte
}

// NewConfigMapDataEditor creates a new ConfigMapDataEditor wrapping the given
// .data and .binaryData map pointers.
//
// Either pointer may be nil, in which case the editor allocates a local
// zero-value map for that field. Operations on that field will succeed but
// writes will not propagate back to any external map. Pass non-nil pointers
// (e.g. &cm.Data, &cm.BinaryData) when the changes must be reflected on the
// object. The maps the pointers refer to may themselves be nil; methods that
// write to a map initialise it automatically.
func NewConfigMapDataEditor(data *map[string]string, binaryData *map[string][]byte) *ConfigMapDataEditor {
	if data == nil {
		var d map[string]string
		data = &d
	}
	if binaryData == nil {
		var bd map[string][]byte
		binaryData = &bd
	}
	return &ConfigMapDataEditor{data: data, binaryData: binaryData}
}

// Raw returns the underlying .data map directly, initialising it if necessary.
//
// This is an escape hatch for free-form editing when none of the structured
// methods are sufficient.
func (e *ConfigMapDataEditor) Raw() map[string]string {
	if *e.data == nil {
		*e.data = make(map[string]string)
	}
	return *e.data
}

// RawBinary returns the underlying .binaryData map directly, initialising it if necessary.
//
// This is an escape hatch for free-form editing.
func (e *ConfigMapDataEditor) RawBinary() map[string][]byte {
	if *e.binaryData == nil {
		*e.binaryData = make(map[string][]byte)
	}
	return *e.binaryData
}

// SetBinary sets key to value in .binaryData, initialising the map if necessary.
func (e *ConfigMapDataEditor) SetBinary(key string, value []byte) {
	if *e.binaryData == nil {
		*e.binaryData = make(map[string][]byte)
	}
	(*e.binaryData)[key] = value
}

// RemoveBinary deletes key from .binaryData. It is a no-op if the key does not exist.
func (e *ConfigMapDataEditor) RemoveBinary(key string) {
	delete(*e.binaryData, key)
}

// Set sets key to value in .data, initialising the map if necessary.
func (e *ConfigMapDataEditor) Set(key, value string) {
	if *e.data == nil {
		*e.data = make(map[string]string)
	}
	(*e.data)[key] = value
}

// Remove deletes key from .data. It is a no-op if the key does not exist.
func (e *ConfigMapDataEditor) Remove(key string) {
	delete(*e.data, key)
}

// MergeYAML deep-merges yamlPatch into the existing value stored at key in .data.
//
// Merge semantics:
//   - If both the current value and the patch are YAML mappings, their keys are
//     merged recursively: keys only in the current value are preserved, keys only
//     in the patch are added, and keys present in both are resolved by applying
//     MergeYAML recursively.
//   - For all other types (scalars, sequences, mixed) the patch value wins.
//
// If the key does not yet exist the patch is written as-is.
// Returns an error if either the existing value or the patch is invalid YAML.
func (e *ConfigMapDataEditor) MergeYAML(key, yamlPatch string) error {
	var base interface{}
	if val, ok := (*e.data)[key]; ok && val != "" {
		if err := yaml.Unmarshal([]byte(val), &base); err != nil {
			return fmt.Errorf("configmap entry %q: failed to parse existing value as YAML: %w", key, err)
		}
	}

	var patchObj interface{}
	if err := yaml.Unmarshal([]byte(yamlPatch), &patchObj); err != nil {
		return fmt.Errorf("configmap entry %q: failed to parse YAML patch: %w", key, err)
	}

	merged := deepMergeYAML(base, patchObj)

	out, err := yaml.Marshal(merged)
	if err != nil {
		return fmt.Errorf("configmap entry %q: failed to marshal merged YAML: %w", key, err)
	}

	if *e.data == nil {
		*e.data = make(map[string]string)
	}
	(*e.data)[key] = string(out)

	return nil
}

// deepMergeYAML recursively merges patch into base.
// When both values are string-keyed maps the merge is recursive.
// In all other cases the patch value wins.
func deepMergeYAML(base, patch interface{}) interface{} {
	baseMap, baseIsMap := base.(map[string]interface{})
	patchMap, patchIsMap := patch.(map[string]interface{})

	if baseIsMap && patchIsMap {
		result := make(map[string]interface{}, len(baseMap))
		for k, v := range baseMap {
			result[k] = v
		}
		for k, v := range patchMap {
			if existing, ok := result[k]; ok {
				result[k] = deepMergeYAML(existing, v)
			} else {
				result[k] = v
			}
		}
		return result
	}

	return patch
}
