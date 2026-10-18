// sudoapi: Preserve Claude protocol fields and tool inputs.

package dto

import (
	"bytes"

	"github.com/QuantumNous/new-api/relaykit/common"
)

// UnmarshalJSON preserves tool input integers and fractions inside message
// content. A float64 round trip can change a signed conversation prefix.
func (message *ClaudeMessage) UnmarshalJSON(data []byte) error {
	type messageFields ClaudeMessage
	var decoded messageFields
	if err := common.DecodeJson(bytes.NewReader(data), &decoded); err != nil {
		return err
	}
	*message = ClaudeMessage(decoded)
	return nil
}
