//go:build !windows
// +build !windows

package utils

import (
	"fmt"
	"os"
	"os/user"

	"golang.org/x/sys/unix"
)

func GetOwnerAndGroup(file *os.File) (string, string, error) {
	fi, err := file.Stat()
	if err != nil {
		return "", "", err
	}

	stat, ok := fi.Sys().(*unix.Stat_t)
	if !ok {
		return "", "", fmt.Errorf("Not a unix.Stat_t")
	}

	uid := fmt.Sprintf("%d", stat.Uid)
	gid := fmt.Sprintf("%d", stat.Gid)

	u, err := user.LookupId(uid)
	if err != nil {
		return "", "", err
	}

	g, err := user.LookupGroupId(gid)
	if err != nil {
		return "", "", err
	}

	return u.Username, g.Name, nil
}
