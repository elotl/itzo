package util

import (
	"fmt"
	"strings"
)

// Add an entry to the list of environment variables. Also removes duplicate
// entries, only keeping the last one.
func AddToEnvList(env []string, key string, value string, overwrite bool) []string {
	found := false
	result := make([]string, 0, len(env))
	for i := len(env) - 1; i >= 0; i-- {
		e := env[i]
		items := strings.SplitN(e, "=", 2)
		if items[0] != key {
			// Some other variable, add it to the end result list.
			result = append(result, e)
			continue
		}
		// Found key.
		if !found {
			found = true
			if overwrite {
				result = append(result, fmt.Sprintf("%s=%s", key, value))
			} else {
				result = append(result, e)
			}
		}
	}
	if !found {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}
	return result
}
