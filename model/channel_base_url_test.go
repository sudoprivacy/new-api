// sudoapi: Official Seedance task adaptor.

package model

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/constant"
)

func TestChannelGetBaseURL(t *testing.T) {
	tests := []struct {
		name      string
		typ       int
		baseURL   string
		expectURL string
	}{
		{
			name:      "allows sparse channel type",
			typ:       constant.ChannelTypeSeedance,
			baseURL:   "https://seedance.example.com",
			expectURL: "https://seedance.example.com",
		}, {
			name:      "sparse channel type without custom url",
			typ:       constant.ChannelTypeSeedance,
			baseURL:   "",
			expectURL: "https://ark.cn-beijing.volces.com",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &Channel{
				Type:    test.typ,
				BaseURL: &test.baseURL,
			}
			require.Equal(t, test.expectURL, channel.GetBaseURL())
		})
	}
}
