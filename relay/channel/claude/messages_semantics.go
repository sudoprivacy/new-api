// sudoapi: Preserve caller Messages semantics receipts on native streams.

package claude

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Ordinary JSON responses already carry upstream headers. A native Claude
// stream starts a fresh header map, sometimes after a queue keepalive.
func forwardMessagesSemanticsReceipt(c *gin.Context, h http.Header) {
	const modeHeader = "X-SudoAPI-Messages-Semantics"
	const hashHeader = "X-SudoAPI-Messages-SHA256"
	modes, hashes := h.Values(modeHeader), h.Values(hashHeader)
	if len(modes) != 1 || len(hashes) != 1 {
		return
	}
	mode, hash := modes[0], hashes[0]
	switch mode {
	case "exact", "sdk-managed", "sdk-fork":
	default:
		return
	}
	if len(hash) != 64 || hash != strings.ToLower(hash) {
		return
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return
	}
	if c.Writer.Written() {
		// The HTTP headers are committed. A bounded SSE comment carries the
		// receipt without adding an application event or changing client usage.
		_, _ = fmt.Fprintf(c.Writer, ": sudoapi-messages-semantics %s %s\n\n", mode, hash)
		c.Writer.Flush()
		return
	}
	c.Header(modeHeader, mode)
	c.Header(hashHeader, hash)
}
