package util

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	passwd = `foo:x:1000:1000:Foo Bar,,,:/home/foo:/bin/bash
test123:x:116:122::/home/test123:/bin/false
milpa:x:1001:1001::/opt/milpa:
`
	group = `empty:x:26:
oneuser:x:27:foo
multipleusers:x:29:foo,bar,test
`
)

//func LookupUser(userspec string, lookup UserLookup) (uint32, uint32, error)
func TestLookupPasswdUser(t *testing.T) {
	dir, err := ioutil.TempDir("", "passwd-user-test")
	assert.NoError(t, err)
	defer os.RemoveAll(dir)
	err = os.MkdirAll(dir+"/etc", 0755)
	assert.NoError(t, err)
	err = ioutil.WriteFile(dir+"/etc/passwd", []byte(passwd), 0755)
	assert.NoError(t, err)
	err = ioutil.WriteFile(dir+"/etc/group", []byte(group), 0755)
	assert.NoError(t, err)
	type testcase struct {
		user    string
		uid     uint32
		gid     uint32
		homedir string
		failure bool
	}
	tcs := []testcase{
		{
			user:    "",
			failure: true,
		},
		{
			user:    "foo",
			uid:     1000,
			gid:     1000,
			homedir: "/home/foo",
			failure: false,
		},
		{
			user:    "test123:empty",
			uid:     116,
			gid:     26,
			homedir: "/home/test123",
			failure: false,
		},
		{
			user:    "1001",
			uid:     1001,
			gid:     1001,
			homedir: "/opt/milpa",
			failure: false,
		},
		{
			user:    "1234",
			uid:     1234,
			gid:     0,
			homedir: "",
			failure: false,
		},
		{
			user:    "milpa:111",
			uid:     1001,
			gid:     111,
			homedir: "/opt/milpa",
			failure: false,
		},
		{
			user:    "1234:1111",
			uid:     1234,
			gid:     1111,
			homedir: "",
			failure: false,
		},
	}
	lookup, err := NewPasswdUserLookup(dir)
	assert.NoError(t, err)
	for _, tc := range tcs {
		uid, gid, homedir, err := LookupUser(tc.user, lookup)
		if tc.failure {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err, "test data: %+v", tc)
		}
		assert.Equal(t, tc.uid, uid)
		assert.Equal(t, tc.gid, gid)
		assert.Equal(t, tc.homedir, homedir)
	}
}
