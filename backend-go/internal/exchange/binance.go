// Package exchange — Binance REST client (testnet/mainnet) + paper wallet.
package exchange

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const (
	TestnetBase = "https://testnet.binance.vision/api/v3"
	MainnetBase = "https://api.binance.com/api/v3"
	FeeRate     = 0.001 // 0.1% per side
)

// Candle is one kline row.
type Candle struct {
	OpenTime int64   `json:"open_time"`
	Open     float64 `json:"open"`
	High     float64 `json:"high"`
	Low      float64 `json:"low"`
	Close    float64 `json:"close"`
	Volume   float64 `json:"volume"`
}

// Client wraps public + signed endpoints.
type Client struct {
	Base      string
	APIKey    string
	APISecret string
	HTTP      *http.Client
}

func NewClient(base, key, secret string) *Client {
	return &Client{Base: base, APIKey: key, APISecret: secret, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) get(path string, params url.Values, out any) error {
	u := c.Base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	resp, err := c.HTTP.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s %s: %d %s", path, params.Encode(), resp.StatusCode, truncate(string(body), 200))
	}
	return json.Unmarshal(body, out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Klines fetches candles.
func (c *Client) Klines(symbol, interval string, limit int) ([]Candle, error) {
	raw := [][]any{}
	params := url.Values{
		"symbol":   {symbol},
		"interval": {interval},
		"limit":    {strconv.Itoa(limit)},
	}
	if err := c.get("/klines", params, &raw); err != nil {
		return nil, err
	}
	out := make([]Candle, 0, len(raw))
	for _, k := range raw {
		out = append(out, Candle{
			OpenTime: int64(k[0].(float64)),
			Open:     toF(k[1]),
			High:     toF(k[2]),
			Low:      toF(k[3]),
			Close:    toF(k[4]),
			Volume:   toF(k[5]),
		})
	}
	return out, nil
}

func toF(v any) float64 {
	switch x := v.(type) {
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	case float64:
		return x
	}
	return 0
}

// TickerPrice gets the latest price.
func (c *Client) TickerPrice(symbol string) (float64, error) {
	var out struct {
		Price string `json:"price"`
	}
	if err := c.get("/ticker/price", url.Values{"symbol": {symbol}}, &out); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(out.Price, 64)
}

// ExchangeSymbols returns TRADING symbols with the given quote asset.
func (c *Client) ExchangeSymbols(quote string) ([]string, error) {
	var raw struct {
		Symbols []struct {
			Symbol string `json:"symbol"`
			Status string `json:"status"`
			Quote  string `json:"quoteAsset"`
		} `json:"symbols"`
	}
	if err := c.get("/exchangeInfo", url.Values{}, &raw); err != nil {
		return nil, err
	}
	out := []string{}
	for _, s := range raw.Symbols {
		if s.Status == "TRADING" && s.Quote == quote {
			out = append(out, s.Symbol)
		}
	}
	sort.Strings(out)
	return out, nil
}

// --- signed requests (live mode) -------------------------------------------

func (c *Client) signed(method, path string, params url.Values, out any) error {
	if c.APIKey == "" || c.APISecret == "" {
		return fmt.Errorf("missing API credentials")
	}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("recvWindow", "10000")

	// signature over sorted query string
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	qs := ""
	for i, k := range keys {
		if i > 0 {
			qs += "&"
		}
		qs += url.QueryEscape(k) + "=" + url.QueryEscape(params.Get(k))
	}
	mac := hmac.New(sha256.New, []byte(c.APISecret))
	mac.Write([]byte(qs))
	params.Set("signature", hex.EncodeToString(mac.Sum(nil)))

	u := c.Base + path + "?" + params.Encode()
	req, err := http.NewRequest(method, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-MBX-APIKEY", c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: %d %s", path, resp.StatusCode, truncate(string(body), 300))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// MarketBuy places a MARKET buy by quote quantity (live mode).
func (c *Client) MarketBuy(symbol string, quoteQty float64) (map[string]any, error) {
	out := map[string]any{}
	params := url.Values{
		"symbol":        {symbol},
		"side":          {"BUY"},
		"type":          {"MARKET"},
		"quoteOrderQty": {trimF(quoteQty)},
	}
	err := c.signed(http.MethodPost, "/order", params, &out)
	return out, err
}

// MarketSell places a MARKET sell by base quantity (live mode).
func (c *Client) MarketSell(symbol string, baseQty float64) (map[string]any, error) {
	out := map[string]any{}
	params := url.Values{
		"symbol":   {symbol},
		"side":    {"SELL"},
		"type":    {"MARKET"},
		"quantity": {trimF(baseQty)},
	}
	err := c.signed(http.MethodPost, "/order", params, &out)
	return out, err
}

// Account fetches balances (live mode).
func (c *Client) Account() (map[string]any, error) {
	out := map[string]any{}
	err := c.signed(http.MethodGet, "/account", url.Values{}, &out)
	return out, err
}

func trimF(x float64) string {
	s := strconv.FormatFloat(x, 'f', 8, 64)
	for len(s) > 1 && s[len(s)-1] == '0' {
		s = s[:len(s)-1]
	}
	if len(s) > 1 && s[len(s)-1] == '.' {
		s = s[:len(s)-1]
	}
	return s
}
