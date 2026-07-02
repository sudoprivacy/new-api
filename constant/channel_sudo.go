// sudoapi: Official Seedance task adaptor.

package constant

import (
	"github.com/samber/lo"
)

const (
	ChannelTypeSudoBase = 10000
	ChannelTypeSeedance = 10001
)

var ChannelBaseURLs map[int]string

func init() {
	ChannelBaseURLs = lo.SliceToMapI(channelBaseURLs, func(url string, i int) (int, string) { return i, url })

	ChannelTypeNames[ChannelTypeSeedance] = "Seedance"
	ChannelBaseURLs[ChannelTypeSeedance] = "https://ark.cn-beijing.volces.com"
}
