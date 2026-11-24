package sim

// CloneProductionState isolates persistent land rights, care contracts and
// accumulated wear from caller-owned world-pack data.
func CloneProductionState(p ProductionState) ProductionState {
	out := p
	out.WorkerRoster = append([]WorkerRun(nil), p.WorkerRoster...)
	if p.Fields != nil {
		out.Fields = append([]FieldAsset{}, p.Fields...)
	}
	if p.CareAgreements != nil {
		out.CareAgreements = append([]GrowthCareAgreement{}, p.CareAgreements...)
	}
	if p.CareProposals != nil {
		out.CareProposals = append([]GrowthCareAgreement{}, p.CareProposals...)
	}
	if p.ToolWear != nil {
		out.ToolWear = make(map[string]map[string]int64, len(p.ToolWear))
		for owner, wear := range p.ToolWear {
			out.ToolWear[owner] = cloneInt64Map(wear)
		}
	}
	out.EnergyRemainder = cloneInt64Map(p.EnergyRemainder)
	out.YieldCredit = cloneInt64Map(p.YieldCredit)
	out.LaborUsed = cloneInt64Map(p.LaborUsed)
	if p.LivingCare != nil {
		out.LivingCare = make(map[string]LivingCareState, len(p.LivingCare))
		for id, care := range p.LivingCare {
			out.LivingCare[id] = care
		}
	}
	return out
}
