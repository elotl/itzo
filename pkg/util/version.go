package util

var (
	// VERSION is set during the build from the file at milpa/version
	VERSION string = ""
	// GIT_REVISION and GIT_DIRTY are set during build.
	GIT_REVISION string = ""
	GIT_DIRTY    string = ""
)

func Version() string {
	version := VERSION
	if GIT_REVISION != "" {
		version = version + "-" + GIT_REVISION
	}
	return version
}
