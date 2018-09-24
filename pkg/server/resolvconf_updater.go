package server

import (
	"fmt"
	"os"
	"strings"
)

type ResolvConfUpdater interface {
	UpdateSearch(string, string) error
}

type RealResolvConfUpdater struct {
	filepath string
}

func ensureNewline(s string) string {
	if !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}

func (u *RealResolvConfUpdater) UpdateSearch(clusterName, namespace string) error {
	lines, err := readLines(u.filepath)
	if err != nil {
		return err
	}
	keep := make([]string, 0, len(lines))
	for i := range lines {
		if strings.HasPrefix(lines[i], "search") || lines[i] == "" {
			continue
		}
		keep = append(keep, ensureNewline(lines[i]))
	}
	searchLine := fmt.Sprintf("search %s.%s.local\n", namespace, clusterName)
	keep = append(keep, searchLine)
	out, err := os.Create(u.filepath)
	if err != nil {
		return err
	}
	defer out.Close()
	for _, line := range keep {
		if _, err := out.WriteString(line); err != nil {
			return err
		}
	}
	return nil
}

type ResolvConfUpdaterMock struct {
	Updater func(clusterName, namespace string) error
	updated bool
}

func NewResolvConfUpdaterMock() *ResolvConfUpdaterMock {
	u := &ResolvConfUpdaterMock{}
	u.Updater = func(clusterName, namespace string) error {
		u.updated = true
		return nil
	}
	return u
}
