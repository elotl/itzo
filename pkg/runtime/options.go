package runtime

import (
	"net/url"
	"strconv"
	"strings"
)

// This is placed in runtime package,
// because I'll refactor RuntimeService log functions to use this struct.
type LogOptions struct {
	UnitName     string
	Follow       bool
	WithMetadata bool
	LineNum      int
	BytesNum     int
}

func NewLogOptionsFromURL(logUrl *url.URL) (*LogOptions, error) {
	var parsedURL *url.URL
	var err error
	// If the request came from our
	// websocket library, the query params are in the path and
	// r.URL.String() doesn't decode them correctly (they get
	// escaped).  However, if the request came from a standard web
	// client (http.Client) the query params are already parsed
	// out into URL.RawQuery.
	if logUrl.RawQuery != "" {
		parsedURL, err = logUrl.Parse(logUrl.String())
	} else {
		parsedURL, err = logUrl.Parse(logUrl.Path)
	}

	if err != nil {
		return nil, err
	}

	path := strings.TrimPrefix(parsedURL.Path, "/")
	parts := strings.Split(path, "/")
	unit := ""
	if len(parts) > 3 {
		unit = strings.Join(parts[3:], "/")
	}
	q := parsedURL.Query()
	followParam := q.Get("follow")
	follow := false
	if followParam != "" {
		follow = true
	}
	withMetadata := false
	if q.Get("metadata") == "1" {
		withMetadata = true
	}
	n := 0
	numBytes := 0
	lines := q.Get("lines")
	strBytes := q.Get("bytes")
	if lines != "" {
		if i, err := strconv.Atoi(lines); err == nil {
			n = i
		}
	}
	if strBytes != "" {
		if i, err := strconv.Atoi(strBytes); err == nil {
			numBytes = i
		}
	}
	return &LogOptions{
		UnitName:     unit,
		Follow:       follow,
		WithMetadata: withMetadata,
		LineNum:      n,
		BytesNum:     numBytes,
	}, nil
}
