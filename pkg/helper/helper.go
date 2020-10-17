package helper

import "github.com/elotl/itzo/pkg/util"

const (
	maximumHostnameLength = 63
	defaultPath           = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

func EnsureDefaultEnviron(env []string, podname, homedir string) []string {
	// Make user HOME, HOSTNAME, PATH and TERM are set (same variables Docker
	// ensures are set). See
	// https://docs.docker.com/v17.09/engine/reference/run/#env-environment-variables
	// for more information.
	hostname := MakeHostname(podname)
	env = util.AddToEnvList(env, "HOSTNAME", hostname, true)
	env = util.AddToEnvList(env, "TERM", "xterm", false)
	env = util.AddToEnvList(env, "HOME", homedir, false)
	env = util.AddToEnvList(env, "PATH", defaultPath, false)
	return env
}

func MakeHostname(podname string) string {
	noNSName := util.GetNameFromString(podname)
	if len(noNSName) > maximumHostnameLength {
		return noNSName[:maximumHostnameLength]
	}
	return noNSName
}
