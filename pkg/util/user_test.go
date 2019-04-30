package util

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

//func LookupUser(userspec string, lookup UserLookup) (uint32, uint32, error)
func TestLookupUser(t *testing.T) {
	type testcase struct {
		user    string
		lookup  UserLookup
		uid     uint32
		gid     uint32
		homedir string
		err     error
		failure bool
	}
	tcs := []testcase{
		{
			user: "",
			lookup: &FakeUserLookup{
				UidErr: fmt.Errorf("Testing lookup error"),
				GidErr: fmt.Errorf("Testing lookup error"),
			},
			failure: true,
		},
		{
			user: "myuser",
			lookup: &FakeUserLookup{
				Uid:    1,
				UidGid: 1,
			},
			uid:     1,
			gid:     1,
			failure: false,
		},
		{
			user: "myuser:mygroup",
			lookup: &FakeUserLookup{
				Uid: 1,
				Gid: 1,
			},
			uid:     1,
			gid:     1,
			failure: false,
		},
		{
			user: "1001",
			lookup: &FakeUserLookup{
				Uid:    1001,
				UidGid: 1001,
			},
			uid:     1001,
			gid:     1001,
			failure: false,
		},
	}
	for _, tc := range tcs {
		uid, gid, homedir, err := LookupUser(tc.user, tc.lookup)
		if tc.failure {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, tc.uid, uid)
		assert.Equal(t, tc.gid, gid)
		if tc.homedir != "" {
			assert.Equal(t, tc.homedir, homedir)
		}
	}
}
