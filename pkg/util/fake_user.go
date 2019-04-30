package util

import (
	"fmt"
	"os/user"
)

type FakeUserLookup struct {
	Uid     uint32
	UidGid  uint32
	UidErr  error
	Gid     uint32
	GidErr  error
	Homedir string
}

func (ful *FakeUserLookup) Lookup(username string) (*user.User, error) {
	usr := user.User{}
	usr.Uid = fmt.Sprintf("%d", ful.Uid)
	usr.Gid = fmt.Sprintf("%d", ful.UidGid)
	return &usr, ful.UidErr
}

func (ful *FakeUserLookup) LookupId(username string) (*user.User, error) {
	usr := user.User{}
	usr.Uid = fmt.Sprintf("%d", ful.Uid)
	usr.Gid = fmt.Sprintf("%d", ful.UidGid)
	return &usr, ful.UidErr
}

func (ful *FakeUserLookup) LookupGroup(name string) (*user.Group, error) {
	grp := user.Group{}
	grp.Gid = fmt.Sprintf("%d", ful.Gid)
	return &grp, ful.UidErr
}

func (ful *FakeUserLookup) LookupGroupId(name string) (*user.Group, error) {
	grp := user.Group{}
	grp.Gid = fmt.Sprintf("%d", ful.Gid)
	return &grp, ful.UidErr
}
