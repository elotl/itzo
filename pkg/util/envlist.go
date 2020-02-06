package util

import (
	"fmt"
	"strings"

	"github.com/elotl/itzo/pkg/api"
	"k8s.io/kubernetes/third_party/forked/golang/expansion"
)

// Add an entry to the list of environment variables. Also removes duplicate
// entries, only keeping the last one.
func AddToEnvList(env []string, key string, value string, overwrite bool) []string {
	found := false
	result := make([]string, 0, len(env))
	for i := len(env) - 1; i >= 0; i-- {
		e := env[i]
		items := strings.SplitN(e, "=", 2)
		if items[0] != key {
			// Some other variable, add it to the end result list.
			result = append(result, e)
			continue
		}
		// Found key.
		if !found {
			found = true
			if overwrite {
				result = append(result, fmt.Sprintf("%s=%s", key, value))
			} else {
				result = append(result, e)
			}
		}
	}
	if !found {
		result = append(result, fmt.Sprintf("%s=%s", key, value))
	}
	return result
}

// V1EnvVarsToMap constructs a map of environment name to value from a slice
// of env vars.
func APIEnvVarsToMap(envs []api.EnvVar) map[string]string {
	result := map[string]string{}
	for _, env := range envs {
		result[env.Name] = env.Value
	}

	return result
}

// ExpandContainerCommandOnlyStatic substitutes only static environment variable values from the
// container environment definitions. This does *not* include valueFrom substitutions.
// TODO: callers should use ExpandContainerCommandAndArgs with a fully resolved list of environment.
func ExpandContainerCommandOnlyStatic(containerCommand []string, envs []api.EnvVar) (command []string) {
	mapping := expansion.MappingFuncFor(APIEnvVarsToMap(envs))
	if len(containerCommand) != 0 {
		for _, cmd := range containerCommand {
			command = append(command, expansion.Expand(cmd, mapping))
		}
	}
	return command
}

func EnvironToAPIEnvVar(envs []string) []api.EnvVar {
	apiEnv := make([]api.EnvVar, 0, len(envs))
	for i := range envs {
		items := strings.SplitN(envs[i], "=", 2)
		apiEnv = append(apiEnv, api.EnvVar{Name: items[0], Value: items[1]})
	}
	return apiEnv
}
