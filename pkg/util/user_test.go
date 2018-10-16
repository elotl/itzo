package util

import (
	"fmt"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestUserLookup struct {
	uid    uint32
	uidGid uint32
	uidErr error
	gid    uint32
	gidErr error
}

func (tul *TestUserLookup) Lookup(username string) (*user.User, error) {
	usr := user.User{}
	usr.Uid = fmt.Sprintf("%d", tul.uid)
	usr.Gid = fmt.Sprintf("%d", tul.uidGid)
	return &usr, tul.uidErr
}

func (tul *TestUserLookup) LookupId(username string) (*user.User, error) {
	usr := user.User{}
	usr.Uid = fmt.Sprintf("%d", tul.uid)
	usr.Gid = fmt.Sprintf("%d", tul.uidGid)
	return &usr, tul.uidErr
}

func (tul *TestUserLookup) LookupGroup(name string) (*user.Group, error) {
	grp := user.Group{}
	grp.Gid = fmt.Sprintf("%d", tul.gid)
	return &grp, tul.uidErr
}

func (tul *TestUserLookup) LookupGroupId(name string) (*user.Group, error) {
	grp := user.Group{}
	grp.Gid = fmt.Sprintf("%d", tul.gid)
	return &grp, tul.uidErr
}

//func LookupUser(userspec string, lookup UserLookup) (uint32, uint32, error)
func TestLookupUser(t *testing.T) {
	type testcase struct {
		user    string
		lookup  UserLookup
		uid     uint32
		gid     uint32
		err     error
		failure bool
	}
	tcs := []testcase{
		{
			user: "",
			lookup: &TestUserLookup{
				uidErr: fmt.Errorf("Testing lookup error"),
				gidErr: fmt.Errorf("Testing lookup error"),
			},
			failure: true,
		},
		{
			user: "myuser",
			lookup: &TestUserLookup{
				uid:    1,
				uidGid: 1,
			},
			uid:     1,
			gid:     1,
			failure: false,
		},
		{
			user: "myuser:mygroup",
			lookup: &TestUserLookup{
				uid: 1,
				gid: 1,
			},
			uid:     1,
			gid:     1,
			failure: false,
		},
		{
			user: "1001",
			lookup: &TestUserLookup{
				uid:    1001,
				uidGid: 1001,
			},
			uid:     1001,
			gid:     1001,
			failure: false,
		},
	}
	for _, tc := range tcs {
		uid, gid, err := LookupUser(tc.user, tc.lookup)
		if tc.failure {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
		assert.Equal(t, tc.uid, uid)
		assert.Equal(t, tc.gid, gid)
	}
}
