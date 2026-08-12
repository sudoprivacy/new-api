// sudoapi: Multi-domain server address resolution.

package system_setting

import (
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

type serverAddressSnapshot struct {
	defaultAddress string
	addresses      map[string]struct{}
}

var serverAddresses atomic.Pointer[serverAddressSnapshot]

func init() {
	serverAddresses.Store(&serverAddressSnapshot{
		defaultAddress: ServerAddress,
		addresses:      map[string]struct{}{ServerAddress: {}},
	})
}

func SetServerAddress(value string) {
	snapshot := &serverAddressSnapshot{addresses: make(map[string]struct{})}
	for _, address := range strings.Split(value, ",") {
		address = strings.TrimRight(strings.TrimSpace(address), "/")
		if address == "" {
			continue
		}
		if snapshot.defaultAddress == "" {
			snapshot.defaultAddress = address
		}
		snapshot.addresses[address] = struct{}{}
	}
	ServerAddress = snapshot.defaultAddress
	serverAddresses.Store(snapshot)
}

func GetServerAddress(c *gin.Context) string {
	snapshot := serverAddresses.Load()
	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = c.Request.URL.Scheme
		}
	}
	if scheme != "http" && scheme != "https" {
		return snapshot.defaultAddress
	}

	host := c.GetHeader("X-Forwarded-Host")
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}

	address := scheme + "://" + host
	if _, ok := snapshot.addresses[address]; ok {
		return address
	}
	return snapshot.defaultAddress
}
