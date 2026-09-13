// Package ta — technical indicators (same math as the Python version).
package ta

// SMA returns the simple moving average of the last period values.
func SMA(values []float64, period int) (float64, bool) {
	n := len(values)
	if period <= 0 || n < period {
		return 0, false
	}
	sum := 0.0
	for _, v := range values[n-period:] {
		sum += v
	}
	return sum / float64(period), true
}

// EMA returns the exponential moving average.
func EMA(values []float64, period int) (float64, bool) {
	n := len(values)
	if period <= 0 || n < period {
		return 0, false
	}
	k := 2.0 / (float64(period) + 1)
	e := 0.0
	for i := 0; i < period; i++ {
		e += values[i]
	}
	e /= float64(period)
	for _, v := range values[period:] {
		e = v*k + e*(1-k)
	}
	return e, true
}

// RSI returns Wilder-smoothed RSI.
func RSI(values []float64, period int) (float64, bool) {
	if len(values) < period+1 {
		return 0, false
	}
	gains := make([]float64, 0, len(values))
	losses := make([]float64, 0, len(values))
	for i := 1; i < len(values); i++ {
		d := values[i] - values[i-1]
		if d > 0 {
			gains = append(gains, d)
			losses = append(losses, 0)
		} else {
			gains = append(gains, 0)
			losses = append(losses, -d)
		}
	}
	avgGain, avgLoss := 0.0, 0.0
	for i := 0; i < period; i++ {
		avgGain += gains[i]
		avgLoss += losses[i]
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)
	p := float64(period)
	for i := period; i < len(gains); i++ {
		avgGain = (avgGain*(p-1) + gains[i]) / p
		avgLoss = (avgLoss*(p-1) + losses[i]) / p
	}
	if avgLoss == 0 {
		return 100, true
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs), true
}

// MACD returns (macdLine, signalLine, histogram).
func MACD(values []float64, fast, slow, signal int) (float64, float64, float64, bool) {
	n := len(values)
	if n < slow+signal {
		return 0, 0, 0, false
	}
	kf := 2.0 / (float64(fast) + 1)
	ks := 2.0 / (float64(slow) + 1)
	ksig := 2.0 / (float64(signal) + 1)

	ef, es := 0.0, 0.0
	for i := 0; i < fast; i++ {
		ef += values[i]
	}
	ef /= float64(fast)
	for i := 0; i < slow; i++ {
		es += values[i]
	}
	es /= float64(slow)

	var efSeries, esSeries []float64
	efSeries = append(efSeries, ef)
	for _, v := range values[fast:] {
		ef = v*kf + ef*(1-kf)
		efSeries = append(efSeries, ef)
	}
	esSeries = append(esSeries, es)
	for _, v := range values[slow:] {
		es = v*ks + es*(1-ks)
		esSeries = append(esSeries, es)
	}
	m := len(efSeries)
	if len(esSeries) < m {
		m = len(esSeries)
	}
	ef2, es2 := efSeries[len(efSeries)-m:], esSeries[len(esSeries)-m:]

	macdLine := make([]float64, m)
	for i := 0; i < m; i++ {
		macdLine[i] = ef2[i] - es2[i]
	}
	if m < signal {
		return 0, 0, 0, false
	}
	sig := 0.0
	for i := 0; i < signal; i++ {
		sig += macdLine[i]
	}
	sig /= float64(signal)
	for _, v := range macdLine[signal:] {
		sig = v*ksig + sig*(1-ksig)
	}
	last := macdLine[m-1]
	return last, sig, last - sig, true
}

// ATR returns Wilder-smoothed average true range.
func ATR(highs, lows, closes []float64, period int) (float64, bool) {
	n := len(closes)
	if len(highs) != n || len(lows) != n || n < period+1 {
		return 0, false
	}
	trs := make([]float64, 0, n-1)
	for i := 1; i < n; i++ {
		tr := highs[i] - lows[i]
		if v := highs[i] - closes[i-1]; v > tr {
			tr = v
		}
		if v := lows[i] - closes[i-1]; -v > tr {
			tr = -v
		}
		trs = append(trs, tr)
	}
	a := 0.0
	for i := 0; i < period; i++ {
		a += trs[i]
	}
	a /= float64(period)
	p := float64(period)
	for i := period; i < len(trs); i++ {
		a = (a*(p-1) + trs[i]) / p
	}
	return a, true
}
