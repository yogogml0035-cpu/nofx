package coinank

import (
	"context"
	"encoding/json"
	"nofx/provider/coinank/coinank_enum"
	"os"
	"testing"
	"time"
)

func TestNetPositions(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run")
	}
	client := NewCoinankClient(coinank_enum.MainUrl, TestApikey)
	resp, err := client.NetPositions(context.TODO(), coinank_enum.Binance, "BTCUSDT", coinank_enum.Hour1, time.Now().UnixMilli(), 10)
	if err != nil {
		t.Skipf("%v", err)
	}
	if len(resp) == 0 {
		t.Skip("empty response")
	}
	if resp[0].Begin <= 0 {
		t.Errorf("begin timestamp error")
	}
	res, err := json.Marshal(resp)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%s", res)
}
