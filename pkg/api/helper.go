package api

func IsHostNetwork(securityContext *PodSecurityContext) bool {
	if securityContext == nil {
		return false
	}
	if securityContext.NamespaceOptions == nil ||
		securityContext.NamespaceOptions.Network != NamespaceModeNode {
		return false
	}
	return true
}
