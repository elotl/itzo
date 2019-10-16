package api

func IsHostNetwork(securityContext *PodSecurityContext) bool {
	if securityContext == nil {
		return false
	}
	if securityContext.NamespaceOptions.Network != NamespaceMode_NODE {
		return false
	}
	return true
}
