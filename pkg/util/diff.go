package util

import (
	"reflect"
)

func MapDiff(spec, status map[string]interface{}) ([]string, []string, []string) {
	// Go through status, things in status but not in spec get added
	// to delete.  Go through spec, things not in status or in status
	// but not equal, get upserted.
	add := make([]string, 0)
	update := make([]string, 0)
	delete := make([]string, 0)
	for k, _ := range status {
		_, exists := spec[k]
		if !exists {
			delete = append(delete, k)
		}
	}
	for k, v := range spec {
		status_val, exists := status[k]
		if !exists {
			add = append(add, k)
		} else if !reflect.DeepEqual(v, status_val) {
			update = append(update, k)
		}
	}
	return add, update, delete
}
