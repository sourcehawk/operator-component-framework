package utils

// PreserveMap merges keys from current into applied only if they are missing in applied.
func PreserveMap(applied, current map[string]string) map[string]string {
	if len(current) == 0 {
		return applied
	}
	if applied == nil {
		applied = make(map[string]string)
	}
	for k, v := range current {
		if _, exists := applied[k]; !exists {
			applied[k] = v
		}
	}
	return applied
}
