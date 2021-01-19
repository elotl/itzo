package mac

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParseImageUrl(t *testing.T) {
	testCases := []struct {
		name        string
		inputUrl    string
		expectedUrl string
	}{
		{
			name:        "no port no proto",
			inputUrl:    "registry.default.svc/vm-id",
			expectedUrl: "http://registry.default.svc/registry/v2/vm?id=vm-id",
		},
		{
			name:        "port no proto",
			inputUrl:    "registry.default.svc:8089/vm-id",
			expectedUrl: "http://registry.default.svc:8089/registry/v2/vm?id=vm-id",
		},
		{
			name:        "port proto",
			inputUrl:    "http://registry.default.svc:8089/vm-id",
			expectedUrl: "http://registry.default.svc:8089/registry/v2/vm?id=vm-id",
		},
		{
			name:        "no port proto",
			inputUrl:    "http://registry.default.svc/vm-id",
			expectedUrl: "http://registry.default.svc/registry/v2/vm?id=vm-id",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gotVMId, got := parseImageUrl(testCase.inputUrl)
			assert.Equal(t, testCase.expectedUrl, got)
			assert.Equal(t, "vm-id", gotVMId)
		})
	}
}
