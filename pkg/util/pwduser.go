package util

import (
	"fmt"
	"os/user"
	"strconv"

	pwduser "github.com/opencontainers/runc/libcontainer/user"
)

type PasswdUserLookup struct {
	RootFs string
	users  map[string]pwduser.User
	uids   map[int]pwduser.User
	groups map[string]pwduser.Group
	gids   map[int]pwduser.Group
}

func NewPasswdUserLookup(path string) (*PasswdUserLookup, error) {
	users, err := pwduser.ParsePasswdFile(path + "/etc/passwd")
	if err != nil {
		return nil, err
	}
	groups, err := pwduser.ParseGroupFile(path + "/etc/group")
	if err != nil {
		return nil, err
	}
	userMap := make(map[string]pwduser.User)
	uidMap := make(map[int]pwduser.User)
	for _, u := range users {
		userMap[u.Name] = u
		uidMap[u.Uid] = u
	}
	groupMap := make(map[string]pwduser.Group)
	gidMap := make(map[int]pwduser.Group)
	for _, g := range groups {
		groupMap[g.Name] = g
		gidMap[g.Gid] = g
	}
	lookup := PasswdUserLookup{
		RootFs: path,
		users:  userMap,
		uids:   uidMap,
		groups: groupMap,
		gids:   gidMap,
	}
	return &lookup, nil
}

func (l *PasswdUserLookup) Lookup(username string) (*user.User, error) {
	entry, found := l.users[username]
	if !found {
		return nil, fmt.Errorf("No such user %q in %q", username, l.RootFs)
	}
	usr := user.User{
		Username: entry.Name,
		Uid:      fmt.Sprintf("%d", entry.Uid),
		Gid:      fmt.Sprintf("%d", entry.Gid),
		HomeDir:  entry.Home,
	}
	return &usr, nil
}

func (l *PasswdUserLookup) LookupId(username string) (*user.User, error) {
	i, err := strconv.ParseInt(username, 10, 32)
	if err != nil {
		return nil, err
	}
	if i < 0 {
		return nil, fmt.Errorf("Invalid user ID %q", username)
	}
	usr := user.User{
		Uid: fmt.Sprintf("%d", i),
	}
	entry, found := l.uids[int(i)]
	if !found {
		return &usr, nil
	}
	usr.Username = entry.Name
	usr.Uid = fmt.Sprintf("%d", entry.Uid)
	usr.Gid = fmt.Sprintf("%d", entry.Gid)
	usr.HomeDir = entry.Home
	return &usr, nil
}

func (l *PasswdUserLookup) LookupGroup(name string) (*user.Group, error) {
	entry, found := l.groups[name]
	if !found {
		return nil, fmt.Errorf("No such group %q in %q", name, l.RootFs)
	}
	grp := user.Group{
		Name: entry.Name,
		Gid:  fmt.Sprintf("%d", entry.Gid),
	}
	return &grp, nil
}

func (l *PasswdUserLookup) LookupGroupId(name string) (*user.Group, error) {
	i, err := strconv.ParseInt(name, 10, 32)
	if err != nil {
		return nil, err
	}
	if i < 0 {
		return nil, fmt.Errorf("Invalid group ID %q", name)
	}
	grp := user.Group{
		Gid: fmt.Sprintf("%d", i),
	}
	entry, found := l.uids[int(i)]
	if !found {
		return &grp, nil
	}
	grp.Name = entry.Name
	grp.Gid = fmt.Sprintf("%d", entry.Gid)
	return &grp, nil
}
