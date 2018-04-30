package server

import (
	"fmt"
	"net/http"
	"strings"
)

func serverError(w http.ResponseWriter, err error) {
	msg := fmt.Sprintf("500 Server Error: %s", err)
	http.Error(w, msg, http.StatusInternalServerError)
}

func badRequest(w http.ResponseWriter, errMsg string) {
	msg := fmt.Sprintf("400 Bad Request: %s", errMsg)
	http.Error(w, msg, http.StatusBadRequest)
}

func getURLPart(i int, path string) (string, error) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if i < 1 || i > len(parts) {
		return "", fmt.Errorf("Could not find part %d of url", i)
	}
	return parts[i-1], nil
}

func getUnitFromPath(path string) string {
	// The path is always /rest/v1/<endpoint>/<unit> for unit-specific
	// endpoints.
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	unit := ""
	if len(parts) > 3 {
		unit = strings.Join(parts[3:], "/")
	}
	return unit
}
