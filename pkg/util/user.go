package util

import (
	"fmt"
	"os/user"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

type UserLookup interface {
	Lookup(username string) (*user.User, error)
	LookupId(username string) (*user.User, error)
	LookupGroup(name string) (*user.Group, error)
	LookupGroupId(name string) (*user.Group, error)
}

type OsUserLookup struct{}

func (oul *OsUserLookup) Lookup(username string) (*user.User, error) {
	return user.Lookup(username)
}

func (oul *OsUserLookup) LookupGroup(name string) (*user.Group, error) {
	return user.LookupGroup(name)
}

func (oul *OsUserLookup) LookupId(username string) (*user.User, error) {
	usr, err := user.LookupId(username)
	if err == nil {
		return usr, err
	}
	i, err := strconv.ParseInt(username, 10, 32)
	if err != nil {
		return nil, err
	}
	if i < 0 {
		return nil, fmt.Errorf("Invalid user ID %q", username)
	}
	// User is not in /etc/passwd, but it's a valid integer as an UID.
	homedir := "/"
	if username == "0" {
		homedir = "/root"
	}
	usr = &user.User{
		Username: username,
		Uid:      username,
		Gid:      username,
		HomeDir:  homedir,
	}
	return usr, nil
}

func (oul *OsUserLookup) LookupGroupId(name string) (*user.Group, error) {
	grp, err := user.LookupGroupId(name)
	if err == nil {
		return grp, err
	}
	i, err := strconv.ParseInt(name, 10, 32)
	if err != nil {
		return nil, err
	}
	if i < 0 {
		return nil, fmt.Errorf("Invalid group ID %q", name)
	}
	// Group is not in /etc/group, but it's a valid integer as a GID.
	grp = &user.Group{
		Gid: name,
	}
	return grp, nil
}

func LookupUser(userspec string, lookup UserLookup) (uint32, uint32, string, error) {
	gidStr := ""
	userName := userspec
	if strings.Contains(userspec, ":") {
		parts := strings.SplitN(userspec, ":", 2)
		userName = parts[0]
		groupName := parts[1]
		grp, err := lookup.LookupGroupId(groupName)
		if err != nil {
			glog.Errorf("Failed to look up GID %s: %v; retrying as groupname",
				groupName, err)
			grp, err = lookup.LookupGroup(groupName)
			if err != nil {
				glog.Errorf("Failed to look up group %s: %v", groupName, err)
				return 0, 0, "", err
			}
		}
		gidStr = grp.Gid
	}
	usr, err := lookup.LookupId(userName)
	if err != nil {
		glog.Errorf("Failed to look up UID %s: %v; retrying as username",
			userName, err)
		usr, err = lookup.Lookup(userName)
		if err != nil {
			glog.Errorf("Failed to look up user %s: %v", userName, err)
			return 0, 0, "", err
		}
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		glog.Errorf("Failed to parse user id %s: %v", usr.Uid, err)
		return 0, 0, "", err
	}
	if gidStr == "" {
		gidStr = usr.Gid
		if gidStr == "" {
			gidStr = "0"
		}
	}
	gid, err := strconv.Atoi(gidStr)
	if err != nil {
		glog.Errorf("Failed to parse group id %s %v", gidStr, err)
		return 0, 0, "", err
	}
	glog.Infof("Using uid %d gid %d", uid, gid)
	return uint32(uid), uint32(gid), usr.HomeDir, nil
}
