// sudoapi: Reconciliation between this gateway's billing and a channel's.

package model

// GatewayChargeRow is one consume log row reduced to what reconciliation needs:
// what a request earned, in quota units, and which channel served it.
type GatewayChargeRow struct {
	RequestId string `gorm:"column:request_id"`
	ChannelId int    `gorm:"column:channel_id"`
	Quota     int    `gorm:"column:quota"`
}

// GetGatewayChargesInWindow returns the consume-log rows for the given channels
// between two timestamps, half-open as [start, end).
//
// Rows without a request id are skipped: they predate the correlation id, or the
// request never carried one, so there is nothing to join them on and reporting
// them would only manufacture unmatched entries.
func GetGatewayChargesInWindow(startTimestamp, endTimestamp int64, channelIds []int) ([]GatewayChargeRow, error) {
	if len(channelIds) == 0 {
		return nil, nil
	}
	var rows []GatewayChargeRow
	err := LOG_DB.Model(&Log{}).
		Select("request_id", "channel_id", "quota").
		Where("type = ?", LogTypeConsume).
		Where("channel_id IN ?", channelIds).
		Where("created_at >= ?", startTimestamp).
		Where("created_at < ?", endTimestamp).
		Where("request_id <> ?", "").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
