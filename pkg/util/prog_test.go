/*
Copyright 2020 Elotl Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func randStr(t *testing.T) string {
	n, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	assert.NoError(t, err)
	return strconv.FormatUint(n.Uint64(), 36)
}

func createScript(t *testing.T, fname, script string) (string, func()) {
	dir, err := ioutil.TempDir("", "itzo-test-")
	assert.NoError(t, err)
	name := filepath.Join(dir, fname)
	content := []byte("#!/bin/sh\n\n" + script + "\n")
	err = ioutil.WriteFile(name, content, 0755)
	assert.NoError(t, err)
	return name, func() { os.RemoveAll(dir) }
}

//func EnsureProg(prog, downloadURL, minVersion, versionArg string) (string, error)
func TestEnsureProg(t *testing.T) {
	testCases := []struct {
		Script         string
		MinimumVersion string
		Failure        bool
	}{
		{
			Script:         "echo v1.0.0",
			MinimumVersion: "v1.0.0",
			Failure:        false,
		},
		{
			Script:         "echo v1.0.0",
			MinimumVersion: "v0.9.11-asdf+g11",
			Failure:        false,
		},
		{
			Script:         "echo v1.0.0",
			MinimumVersion: "v1.0.1",
			Failure:        true,
		},
		{
			Script:         "echo v0.3.2-beta.1-24-g4e13342-dirty",
			MinimumVersion: "v0.3.4",
			Failure:        true,
		},
		{
			Script:         "echo v0.3.2-beta.1-24-g4e13342-dirty",
			MinimumVersion: "v0.3.1",
			Failure:        false,
		},
		{
			Script:         "exit 0",
			MinimumVersion: "v0.3.1",
			Failure:        true,
		},
		{
			Script:         "echo program version 1.0.0",
			MinimumVersion: "1.0.0",
			Failure:        false,
		},
		{
			Script:         "echo program version 1.0.0, built on 202005121109",
			MinimumVersion: "1.0.0",
			Failure:        false,
		},
	}
	for _, tc := range testCases {
		path, closer := createScript(t, randStr(t), tc.Script)
		defer closer()
		foundPath, err := EnsureProg(
			path,
			"https://whatever.lkj.asdf",
			tc.MinimumVersion,
			"--version")
		if tc.Failure {
			msg := fmt.Sprintf("Test case %+v not failed as expected", tc)
			assert.Error(t, err, msg)
			assert.NotEqual(t, path, foundPath, msg)
		} else {
			msg := fmt.Sprintf("Test case %+v failed", tc)
			assert.NoError(t, err, msg)
			assert.Equal(t, path, foundPath, msg)
		}
	}
}

//func RunProg(prog string, outputLimit, maxRetries int, args ...string) error
func TestRunProg(t *testing.T) {
	testCases := []struct {
		Script  string
		Args    []string
		Failure bool
	}{
		{
			Script:  "echo $@",
			Args:    []string{"1", "2", "3"},
			Failure: false,
		},
		{
			Script:  "exit 0",
			Args:    nil,
			Failure: false,
		},
		{
			Script:  "exit 1",
			Args:    nil,
			Failure: true,
		},
		{
			Script:  "echo foobar; exit 1",
			Args:    nil,
			Failure: true,
		},
	}
	for _, tc := range testCases {
		path, closer := createScript(t, randStr(t), tc.Script)
		defer closer()
		err := RunProg(path, 64, 1, tc.Args...)
		if tc.Failure {
			msg := fmt.Sprintf("Test case %+v not failed as expected", tc)
			assert.Error(t, err, msg)
		} else {
			msg := fmt.Sprintf("Test case %+v failed", tc)
			assert.NoError(t, err, msg)
		}
	}
}
