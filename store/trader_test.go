package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// show_in_competition / is_cross_margin 不能带 gorm default:true：带 default 的 bool 在 Create
// 时会把明确的 false 当零值省略，数据库默认值改回 true，用户选的“隐藏/逐仓”就丢了。
func TestCreateTraderPersistsFalseBooleans(t *testing.T) {
	st, err := New(filepath.Join(t.TempDir(), "trader.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	trader := &Trader{
		ID:                "trader-1",
		UserID:            "user-1",
		Name:              "hidden isolated",
		AIModelID:         "model-1",
		ExchangeID:        "exchange-1",
		ShowInCompetition: false,
		IsCrossMargin:     false,
	}
	require.NoError(t, st.Trader().Create(trader))

	stored, err := st.Trader().GetByID(trader.ID)
	require.NoError(t, err)
	assert.Equal(t, "user-1", stored.UserID)
	assert.False(t, stored.ShowInCompetition)
	assert.False(t, stored.IsCrossMargin)
}
