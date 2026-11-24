package main

// 精确定位 amend 拒绝边界：order_id 放请求体（int64 / 字符串）× 是否带自定义 text
import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gate/gateapi-go/v7"

	nofxgate "nofx/trader/gate"
)

const contract = "DOGE_USDT"
const base = "https://api.gateio.ws"

func sign(method, url, query, body, timestamp string) string {
	hash := sha512.Sum512([]byte(body))
	mac := hmac.New(sha512.New, []byte(os.Getenv("GATE_SECRET")))
	mac.Write([]byte(method + "\n" + url + "\n" + query + "\n" + hex.EncodeToString(hash[:]) + "\n" + timestamp))
	return hex.EncodeToString(mac.Sum(nil))
}

func rawRequest(method, pathWithQuery, body string) (int, string) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, _ := http.NewRequest(method, base+pathWithQuery, bytes.NewBufferString(body))
	req.Header.Set("Key", os.Getenv("GATE_KEY"))
	req.Header.Set("Timestamp", ts)
	req.Header.Set("SIGN", sign(method, pathWithQuery, "", body, ts))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func place(gt *nofxgate.GateTrader, text string, price float64) string {
	plain := gateapi.FuturesPriceTriggeredOrder{
		Initial: gateapi.FuturesInitialOrder{Contract: contract, Size: 1, Price: "0", Tif: "ioc", Text: text},
		Trigger: gateapi.FuturesPriceTrigger{Price: fmt.Sprintf("%.5f", price*0.5), Rule: nofxgate.PriceRuleLE},
	}
	id, err := gt.PlaceProtectiveOrder(plain)
	if err != nil {
		fmt.Println("place:", err)
		os.Exit(1)
	}
	return id
}

func main() {
	gt := nofxgate.NewGateTrader(os.Getenv("GATE_KEY"), os.Getenv("GATE_SECRET"))
	price, err := gt.GetMarketPrice(contract)
	if err != nil {
		fmt.Println("price:", err)
		os.Exit(1)
	}
	newTrigger := fmt.Sprintf("%.5f", price*0.45)

	run := func(name, text string, body map[string]any) {
		id := place(gt, text, price)
		idNum, _ := strconv.ParseInt(id, 10, 64)
		if _, ok := body["order_id"]; ok && body["order_id"] == "NUM" {
			body["order_id"] = idNum
		}
		if _, ok := body["order_id"]; ok && body["order_id"] == "STR" {
			body["order_id"] = id
		}
		raw, _ := json.Marshal(body)
		status, text := rawRequest(http.MethodPut, "/api/v4/futures/usdt/price_orders/amend", string(raw))
		fmt.Printf("%s: id=%s status=%d resp=%s\n", name, id, status, text)
		if err := gt.CancelProtectiveOrder(id); err != nil {
			fmt.Println("cleanup cancel failed:", id, err)
			os.Exit(1)
		}
	}

	run("T5_no_text_body_int_id", "", map[string]any{"order_id": "NUM", "trigger_price": newTrigger})
	run("T6_t-nofx_body_int_id", "t-nofx", map[string]any{"order_id": "NUM", "trigger_price": newTrigger})
	run("T7_no_text_body_str_id", "", map[string]any{"order_id": "STR", "trigger_price": newTrigger})
	fmt.Println("DONE")
}
