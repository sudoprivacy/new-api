// sudoapi: Stable errors for relay request validation.

package helper

import "errors"

var ErrMessagesRequired = errors.New("field messages is required")
