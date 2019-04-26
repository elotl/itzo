package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddToEnvList(t *testing.T) {
	testCases := []struct {
		env         []string
		key         string
		value       string
		overwrite   bool
		result      []string
		description string
	}{
		{
			env:         []string{"foo=1", "bar=2"},
			key:         "foo",
			value:       "updated",
			overwrite:   true,
			result:      []string{"foo=updated", "bar=2"},
			description: "Overwrite one variable",
		},
		{
			env:         []string{"foo=1", "bar=2"},
			key:         "something",
			value:       "else",
			overwrite:   false,
			result:      []string{"foo=1", "bar=2", "something=else"},
			description: "Add one variable",
		},
		{
			env:         []string{"foo=1", "bar=2"},
			key:         "foo",
			value:       "updated",
			overwrite:   false,
			result:      []string{"foo=1", "bar=2"},
			description: "Don't overwrite existing variable",
		},
		{
			env:         []string{"foo=1", "bar=2", "foo=3"},
			key:         "foo",
			value:       "updated",
			overwrite:   true,
			result:      []string{"foo=updated", "bar=2"},
			description: "Overwrite one variable and remove duplicates",
		},
		{
			env:         []string{"foo=1", "bar=2", "foo=3"},
			key:         "foo",
			value:       "updated",
			overwrite:   false,
			result:      []string{"foo=3", "bar=2"},
			description: "Don't overwrite variable but remove duplicate",
		},
	}
	for i, tc := range testCases {
		res := AddToEnvList(tc.env, tc.key, tc.value, tc.overwrite)
		description := fmt.Sprintf("Test case %d: %s", i+1, tc.description)
		assert.ElementsMatch(t, tc.result, res, description)
	}
}
