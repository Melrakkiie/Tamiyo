// Package clientip decides which proxies the engine believes about the client IP.
package clientip

import "github.com/gin-gonic/gin"

// Configure makes X-Forwarded-For the only remote IP header and believes it
// only from the trusted proxies (CIDR ranges or IPs); none means the socket
// address is always the client IP.
func Configure(r *gin.Engine, trusted []string) error {
	r.RemoteIPHeaders = []string{"X-Forwarded-For"}
	// SetTrustedProxies(nil) trusts none; an empty non-nil slice behaves the same.
	return r.SetTrustedProxies(trusted)
}
