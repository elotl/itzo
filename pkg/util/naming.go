package util

import "strings"

const (
	NamespaceSeparator = '_'
)

func GetNamespaceFromString(n string) string {
	if i := strings.IndexByte(n, NamespaceSeparator); i > 0 {
		return n[:i]
	}
	return ""
}

func GetNameFromString(n string) string {
	i := strings.IndexByte(n, NamespaceSeparator)
	if i >= 0 && i < len(n)-1 {
		return n[i+1:]
	} else if i == len(n)-1 {
		return ""
	}
	return n
}

func WithNamespace(ns, name string) string {
	return ns + string(NamespaceSeparator) + name
}

func SplitNamespaceAndName(n string) (string, string) {
	parts := strings.SplitN(n, string(NamespaceSeparator), 2)
	if len(parts) == 0 {
		return "", ""
	} else if len(parts) == 1 {
		return "", parts[0]
	} else {
		return parts[0], parts[1]
	}
}
