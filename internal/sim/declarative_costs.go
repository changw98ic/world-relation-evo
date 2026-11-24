package sim

// declarativeCurrencyCosts combines every charge against the same wallet before
// either eligibility checks or settlement. The empty currency ID means coins.
func declarativeCurrencyCosts(spec ActionSpec) map[string]int64 {
	costs := make(map[string]int64, len(spec.CurrencyCosts)+1)
	costs["coins"] = int64(spec.MoneyCost)
	for id, amount := range spec.CurrencyCosts {
		if id == "" {
			id = "coins"
		}
		costs[id] += int64(amount)
	}
	return costs
}
