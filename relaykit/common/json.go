// sudoapi: Preserve Claude protocol fields and tool inputs.

package common

import (
	"encoding/json"
	"io"
)

// DecodeJson keeps JSON numbers exact in provider payloads with arbitrary content.
// This wrapper belongs to relaykit so the module stays independently buildable.
func DecodeJson(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	return decoder.Decode(target)
}
