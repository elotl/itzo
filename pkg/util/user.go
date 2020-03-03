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
			glog.Errorf("Failed to look up GID %q: %v; retrying as groupname",
				groupName, err)
			grp, err = lookup.LookupGroup(groupName)
			if err != nil {
				glog.Errorf("Failed to look up group %q: %v", groupName, err)
				return 0, 0, "", err
			}
		}
		gidStr = grp.Gid
	}
	usr, err := lookup.LookupId(userName)
	if err != nil {
		glog.Errorf("Failed to look up UID %q: %v; retrying as username",
			userName, err)
		usr, err = lookup.Lookup(userName)
		if err != nil {
			glog.Errorf("Failed to look up user %q: %v", userName, err)
			return 0, 0, "", err
		}
	}
	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		err = fmt.Errorf("Failed to parse user id %q: %v", usr.Uid, err)
		glog.Errorf("%v", err)
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
		err = fmt.Errorf("Failed to parse group id %q %v", gidStr, err)
		glog.Errorf("%v", err)
		return 0, 0, "", err
	}
	glog.Infof("Using uid %d gid %d", uid, gid)
	return uint32(uid), uint32(gid), usr.HomeDir, nil
}
