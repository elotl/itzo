package runtime

import (
	"github.com/stretchr/testify/assert"
	"net/url"
	"testing"
)

func TestNewLogOptionsFromURL(t *testing.T) {
	testCases := []struct {
		name               string
		rawUrl             string
		expectError        bool
		expectedLogOptions LogOptions
	}{
		{
			name:        "unit name only",
			rawUrl:      "/rest/v1/logs/unitname",
			expectError: false,
			expectedLogOptions: LogOptions{
				UnitName:     "unitname",
				Follow:       false,
				WithMetadata: false,
				LineNum:      0,
				BytesNum:     0,
			},
		},
		{
			name:        "unit, lines and bytes",
			rawUrl:      "/rest/v1/logs/unitname?lines=10&bytes=5",
			expectError: false,
			expectedLogOptions: LogOptions{
				UnitName:     "unitname",
				Follow:       false,
				WithMetadata: false,
				LineNum:      10,
				BytesNum:     5,
			},
		},
		{
			name:        "unit and lines",
			rawUrl:      "/rest/v1/logs/unitname?lines=10",
			expectError: false,
			expectedLogOptions: LogOptions{
				UnitName:     "unitname",
				Follow:       false,
				WithMetadata: false,
				LineNum:      10,
				BytesNum:     0,
			},
		},
		{
			name:        "unit and lines",
			rawUrl:      "/rest/v1/logs/unitname?bytes=10",
			expectError: false,
			expectedLogOptions: LogOptions{
				UnitName:     "unitname",
				Follow:       false,
				WithMetadata: false,
				LineNum:      0,
				BytesNum:     10,
			},
		},
		{
			name:        "follow and metadata",
			rawUrl:      "/rest/v1/logs/unitname?follow=1&metadata=1",
			expectError: false,
			expectedLogOptions: LogOptions{
				UnitName:     "unitname",
				Follow:       true,
				WithMetadata: true,
				LineNum:      0,
				BytesNum:     0,
			},
		},
		{
			name:        "encoded url",
			rawUrl:      "%2Frest%2Fv1%2Flogs%2Funitname%3Fbytes%3D10%26lines%3D5%26follow%3D1%26metadata%3D1",
			expectError: false,
			expectedLogOptions: LogOptions{
				UnitName:     "unitname",
				Follow:       true,
				WithMetadata: true,
				LineNum:      5,
				BytesNum:     10,
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			logUrl, err := url.Parse(testCase.rawUrl)
			assert.NoError(t, err)
			logOptions, err := NewLogOptionsFromURL(logUrl)
			if testCase.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedLogOptions.UnitName, logOptions.UnitName)
			assert.Equal(t, testCase.expectedLogOptions.Follow, logOptions.Follow)
			assert.Equal(t, testCase.expectedLogOptions.WithMetadata, logOptions.WithMetadata)
			assert.Equal(t, testCase.expectedLogOptions.LineNum, logOptions.LineNum)
			assert.Equal(t, testCase.expectedLogOptions.BytesNum, logOptions.BytesNum)
		})
	}
}
