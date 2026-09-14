// Package exchange — Binance REST client (testnet/mainnet) + paper wallet.
package exchange

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
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

// AllPrices returns every symbol's latest price (USDT valuation of balances).
func (c *Client) AllPrices() (map[string]float64, error) {
	var raw []struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}
	if err := c.get("/ticker/price", url.Values{}, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(raw))
	for _, p := range raw {
		if f, err := strconv.ParseFloat(p.Price, 64); err == nil {
			out[p.Symbol] = f
		}
	}
	return out, nil
}

// Balance is one asset row of the testnet account.
type Balance struct {
	Asset  string
	Free   float64
	Locked float64
}

// ValuedBalance is a balance priced in USDT.
type ValuedBalance struct {
	Asset     string
	Free      float64
	Locked    float64
	Price     float64
	USDTValue float64
}

func toStr(v any) string {
	s, _ := v.(string)
	return s
}

// parseBalances decodes the raw /account "balances" array (drops zero rows).
func parseBalances(raw []any) []Balance {
	out := []Balance{}
	for _, b := range raw {
		bm, _ := b.(map[string]any)
		free, _ := strconv.ParseFloat(toStr(bm["free"]), 64)
		locked, _ := strconv.ParseFloat(toStr(bm["locked"]), 64)
		asset := toStr(bm["asset"])
		if free <= 0 && locked <= 0 {
			continue
		}
		out = append(out, Balance{Asset: asset, Free: free, Locked: locked})
	}
	return out
}

// ValueBalances prices EVERY balance in USDT (USDT = 1; other assets via the
// <ASSET>USDT pair) — testnet seeds the account with many coins, so equity
// must be the total USDT value of all of them. Returns rows sorted by value
// (desc) plus the account total.
func ValueBalances(raw []any, prices map[string]float64) ([]ValuedBalance, float64) {
	rows := []ValuedBalance{}
	total := 0.0
	for _, b := range parseBalances(raw) {
		price := 0.0
		if b.Asset == "USDT" {
			price = 1
		} else if p, ok := prices[b.Asset+"USDT"]; ok {
			price = p
		}
		val := (b.Free + b.Locked) * price
		total += val
		rows = append(rows, ValuedBalance{
			Asset: b.Asset, Free: b.Free, Locked: b.Locked,
			Price: price, USDTValue: val,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].USDTValue > rows[j].USDTValue })
	return rows, total
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

// signaturePayload builds the sorted, percent-encoded query string that the
// HMAC is computed over (exactly the string Binance requires signing).
func signaturePayload(params url.Values) string {
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
	return qs
}

// hmacHex returns the hex-encoded HMAC-SHA256 of payload keyed by secret.
func hmacHex(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// --- signed requests (live mode) -------------------------------------------

func (c *Client) signed(method, path string, params url.Values, out any) error {
	if c.APIKey == "" || c.APISecret == "" {
		return fmt.Errorf("missing API credentials")
	}
	params.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	params.Set("recvWindow", "10000")

	// signature over sorted query string
	qs := signaturePayload(params)
	params.Set("signature", hmacHex(c.APISecret, qs))

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

// --- exchange filters (LOT_SIZE / NOTIONAL) — cần cho convert hết dust ---

var (
	filterMu    sync.Mutex
	lotCache    = map[string]LotFilter{}  // symbol → filter
	exchInfoAt  time.Time
)

// LotFilter holds the tradable step/min/max for one symbol.
type LotFilter struct {
	Symbol    string
	StepSize  float64 // LOT_SIZE stepSize (base units)
	MinQty    float64 // LOT_SIZE minQty
	MaxQty    float64
	MinNotional float64 // NOTIONAL / MIN_NOTIONAL minNotional (quote USDT)
}

// SymbolInfo is one row of /exchangeInfo we care about.
type SymbolInfo struct {
	Symbol  string          `json:"symbol"`
	Status  string          `json:"status"`
	Filters []struct {
		FilterType  string `json:"filterType"`
		StepSize    string `json:"stepSize,omitempty"`
		MinQty      string `json:"minQty,omitempty"`
		MaxQty      string `json:"maxQty,omitempty"`
		MinNotional string `json:"minNotional,omitempty"`
		Notional    string `json:"notional,omitempty"` // newer API shape
	} `json:"filters"`
}

// LoadExchangeInfo fetches + caches LOT_SIZE / NOTIONAL filters for all symbols.
// Cached 30 phút để không spam /exchangeInfo (rất lớn).
func (c *Client) LoadExchangeInfo() error {
	filterMu.Lock()
	defer filterMu.Unlock()
	if time.Since(exchInfoAt) < 30*time.Minute && len(lotCache) > 0 {
		return nil
	}
	var raw struct {
		Symbols []SymbolInfo `json:"symbols"`
	}
	if err := c.get("/exchangeInfo", url.Values{}, &raw); err != nil {
		return err
	}
	cache := map[string]LotFilter{}
	for _, s := range raw.Symbols {
		if s.Status != "TRADING" {
			continue
		}
		lf := LotFilter{Symbol: s.Symbol}
		for _, f := range s.Filters {
			switch f.FilterType {
			case "LOT_SIZE":
				lf.StepSize, _ = strconv.ParseFloat(f.StepSize, 64)
				lf.MinQty, _ = strconv.ParseFloat(f.MinQty, 64)
				lf.MaxQty, _ = strconv.ParseFloat(f.MaxQty, 64)
			case "NOTIONAL", "MIN_NOTIONAL":
				if f.MinNotional != "" {
					lf.MinNotional, _ = strconv.ParseFloat(f.MinNotional, 64)
				} else if f.Notional != "" {
					lf.MinNotional, _ = strconv.ParseFloat(f.Notional, 64)
				}
			}
		}
		cache[s.Symbol] = lf
	}
	lotCache = cache
	exchInfoAt = time.Now()
	return nil
}

// LotSize returns the cached filter for one symbol (zero ok=false nếu chưa load).
func (c *Client) LotSize(symbol string) (LotFilter, bool) {
	filterMu.Lock()
	defer filterMu.Unlock()
	lf, ok := lotCache[symbol]
	return lf, ok
}

// RoundQty làm tròn qty xuống bội số của stepSize (floor), không vượt maxQty.
func (lf LotFilter) RoundQty(qty float64) float64 {
	if lf.StepSize <= 0 {
		return math.Floor(qty*1e8) / 1e8
	}
	steps := math.Floor(qty / lf.StepSize)
	q := steps * lf.StepSize
	// fix lỗi dấu phẩy động: 0.1*3 = 0.30000000000000004
	q = math.Round(q/lf.StepSize) * lf.StepSize
	if lf.MaxQty > 0 && q > lf.MaxQty {
		q = lf.MaxQty
	}
	return q
}

// Sellable kiểm tra 1 balance có bán được không: có pair symbol, qty ≥ minQty,
// và giá trị ≥ minNotional (nếu biết). Trả về symbol + qty được làm tròn.
func (c *Client) Sellable(b ValuedBalance, price float64) (string, float64, bool) {
	if b.Asset == "USDT" || b.Free <= 0 || price <= 0 {
		return "", 0, false
	}
	sym := b.Asset + "USDT"
	if _, err := c.TickerPrice(sym); err != nil {
		return "", 0, false // không có pair
	}
	lf, _ := c.LotSize(sym)
	if lf.MinQty > 0 && b.Free < lf.MinQty {
		return "", 0, false
	}
	qty := b.Free
	if lf.StepSize > 0 {
		qty = lf.RoundQty(b.Free)
	}
	if qty <= 0 {
		return "", 0, false
	}
	if lf.MinNotional > 0 && qty*price < lf.MinNotional {
		return "", 0, false
	}
	return sym, qty, true
}

// HasPair checks whether <ASSET>USDT exists and is TRADING.
func (c *Client) HasPair(asset string) bool {
	_, err := c.TickerPrice(asset + "USDT")
	return err == nil
}

// SellQuoteValue ước tính giá trị quote nhận được khi bán qty (trừ phí 0.1%).
func SellQuoteValue(qty, price float64) float64 {
	return qty * price * (1 - FeeRate)
}

// DustThreshold: giá trị tối thiểu (USDT) để 1 coin được auto-convert —
// dưới ngưỡng này bỏ qua (phí + minNotional làm nó vô nghĩa).
const DustThreshold = 1.0

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
