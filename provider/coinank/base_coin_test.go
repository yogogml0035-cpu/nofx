package coinank

import (
	"context"
	"encoding/json"
	"nofx/provider/coinank/coinank_enum"
	"os"
	"testing"
)

func TestListCoin(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run")
	}
	client := NewCoinankClient(coinank_enum.MainUrl, TestApikey)
	resp, err := client.ListCoin(context.TODO(), "SPOT")
	if err != nil {
		t.Skipf("%v", err)
	}
	res, err := json.Marshal(resp)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%s", res)
}

func TestListSymbols(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set RUN_INTEGRATION_TESTS=1 to run")
	}
	client := NewCoinankClient(coinank_enum.MainUrl, TestApikey)
	resp, err := client.ListSymbols(context.TODO(), "Binance", "SWAP")
	if err != nil {
		t.Skipf("%v", err)
	}
	res, err := json.Marshal(resp)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%s", res)
}
