// sudoapi: Read prompt cache health from the configured log database.

package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCacheHealthReadsSeparateLogDatabase(t *testing.T) {
	mainDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logsDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := DB, LOG_DB
	DB, LOG_DB = mainDB, logsDB
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		for _, db := range []*gorm.DB{mainDB, logsDB} {
			sqlDB, closeErr := db.DB()
			if closeErr == nil {
				_ = sqlDB.Close()
			}
		}
	})
	require.NoError(t, mainDB.AutoMigrate(&Log{}, &Channel{}))
	require.NoError(t, logsDB.AutoMigrate(&Log{}))
	require.NoError(t, mainDB.Create(&Channel{Id: 42, Name: "FujiToken"}).Error)
	// A stale main-database table must not hide current log-database traffic.
	require.NoError(t, logsDB.Create(&Log{
		ChannelId: 42, CreatedAt: 100, Type: LogTypeConsume, ModelName: "claude-opus-5",
		Other: `{"cache_tokens":900,"cache_creation_tokens":100}`,
	}).Error)
	rows, truncated, err := GetChannelCacheHealth(1, 200, 42, "claude", false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, truncated)
	assert.Equal(t, "FujiToken", rows[0].ChannelName)
	assert.Equal(t, int64(1), rows[0].Requests)
	assert.Equal(t, int64(900), rows[0].CacheRead)
	assert.Equal(t, int64(100), rows[0].CacheWrite)
	assert.Equal(t, 90.0, rows[0].ReusePct)
}
