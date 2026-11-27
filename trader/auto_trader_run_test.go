package trader

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 更新保存时 LoadUserTradersFromStore 会按 is_running 自动拉起，handler 随后又显式 Run。
// 第二个 Run 必须直接返回，否则同一个 trader 会有两个循环并发决策。
func TestRunIgnoresDuplicateStart(t *testing.T) {
	at := &AutoTrader{id: "trader-1"}
	at.isRunningMutex.Lock()
	at.isRunning = true
	at.isRunningMutex.Unlock()

	originalStart := at.startTime
	require.NoError(t, at.Run())

	assert.Equal(t, originalStart, at.startTime, "duplicate Run must not reset the loop state")
	at.isRunningMutex.RLock()
	running := at.isRunning
	at.isRunningMutex.RUnlock()
	assert.True(t, running)
}
