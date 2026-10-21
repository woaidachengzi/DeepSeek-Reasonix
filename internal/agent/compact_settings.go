package agent

// CompactRatio returns the fraction of the window at which auto-compaction
// fires (e.g. 0.8). The status line uses it to show headroom to the next compact.
func (a *Agent) CompactRatio() float64 {
	if a.compactRatioSource != nil {
		if ratio := a.compactRatioSource(); ratio > 0 && ratio < 1 {
			return ratio
		}
	}
	return a.compactRatio
}
