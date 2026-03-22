package editors

// SecretDataEditor provides a typed API for mutating the .data and .stringData
// fields of a Kubernetes Secret.
//
// It exposes structured operations (Set, Remove, SetString, RemoveString) as
// well as Raw() and RawStringData() for free-form access when none of the
// structured methods are sufficient.
//
// Note on Kubernetes semantics: the API server merges .stringData into .data
// on write and returns only .data on read. During reconciliation it is safe to
// use either field; the two coexist on the desired object until it is applied.
type SecretDataEditor struct {
	data       *map[string][]byte
	stringData *map[string]string
}

// NewSecretDataEditor creates a new SecretDataEditor wrapping the given .data
// and .stringData map pointers.
//
// Either pointer may be nil, in which case the editor allocates a local
// zero-value map for that field. Operations on that field will succeed but
// writes will not propagate back to any external map. Pass non-nil pointers
// (e.g. &secret.Data, &secret.StringData) when the changes must be reflected
// on the object. The maps the pointers refer to may themselves be nil; methods
// that write to a map initialise it automatically.
func NewSecretDataEditor(data *map[string][]byte, stringData *map[string]string) *SecretDataEditor {
	if data == nil {
		var d map[string][]byte
		data = &d
	}
	if stringData == nil {
		var sd map[string]string
		stringData = &sd
	}
	return &SecretDataEditor{data: data, stringData: stringData}
}

// Raw returns the underlying .data map directly, initialising it if necessary.
//
// This is an escape hatch for free-form editing when none of the structured
// methods are sufficient.
func (e *SecretDataEditor) Raw() map[string][]byte {
	if *e.data == nil {
		*e.data = make(map[string][]byte)
	}
	return *e.data
}

// RawStringData returns the underlying .stringData map directly, initialising
// it if necessary.
//
// This is an escape hatch for free-form editing.
func (e *SecretDataEditor) RawStringData() map[string]string {
	if *e.stringData == nil {
		*e.stringData = make(map[string]string)
	}
	return *e.stringData
}

// Set sets key to value in .data, initialising the map if necessary.
func (e *SecretDataEditor) Set(key string, value []byte) {
	if *e.data == nil {
		*e.data = make(map[string][]byte)
	}
	(*e.data)[key] = value
}

// Remove deletes key from .data. It is a no-op if the key does not exist.
func (e *SecretDataEditor) Remove(key string) {
	delete(*e.data, key)
}

// SetString sets key to value in .stringData, initialising the map if necessary.
//
// The API server will merge .stringData into .data on write; use this when
// working with plaintext values that should not be pre-encoded.
func (e *SecretDataEditor) SetString(key, value string) {
	if *e.stringData == nil {
		*e.stringData = make(map[string]string)
	}
	(*e.stringData)[key] = value
}

// RemoveString deletes key from .stringData. It is a no-op if the key does not exist.
func (e *SecretDataEditor) RemoveString(key string) {
	delete(*e.stringData, key)
}
