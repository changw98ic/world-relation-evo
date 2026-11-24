package sim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// A numeric zero can be deliberate input, or the Go zero value of an omitted field.
func resolveSpecNumber[T int | int64](value, fallback T, explicit bool) T {
	if value != 0 {
		return value
	}
	if explicit {
		return 0
	}
	return fallback
}

func unmarshalSpecNumbers(data []byte, spec any, explicit *map[string]bool) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(spec); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	inputZeros := make(map[string]bool)
	for key, value := range fields {
		if string(value) == "0" || string(value) == "-0" {
			inputZeros[strings.ToLower(key)] = true
		}
	}
	// Copy on write so decoding into a copy cannot change the original spec.
	var zeros map[string]bool
	value := reflect.ValueOf(spec).Elem()
	for i := range value.NumField() {
		field := value.Field(i)
		if field.Kind() != reflect.Int && field.Kind() != reflect.Int64 {
			continue
		}
		key, _, _ := strings.Cut(value.Type().Field(i).Tag.Get("json"), ",")
		if field.Int() < 0 {
			return fmt.Errorf("%s must be nonnegative", key)
		}
		if field.Int() == 0 && (inputZeros[key] || (*explicit)[key]) {
			if zeros == nil {
				zeros = make(map[string]bool)
			}
			zeros[key] = true
		}
	}
	*explicit = zeros
	return nil
}

func marshalSpecNumbers(spec any, explicit map[string]bool, emitImplicitZeros, legacyExplicit bool) ([]byte, error) {
	data, err := json.Marshal(spec)
	if err != nil || len(explicit) == 0 && (!emitImplicitZeros || legacyExplicit) {
		return data, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if emitImplicitZeros && !legacyExplicit {
		for key, value := range fields {
			if string(value) == "0" && !explicit[key] {
				delete(fields, key)
			}
		}
	}
	for key := range explicit {
		if _, present := fields[key]; !present {
			fields[key] = json.RawMessage("0")
		}
	}
	return json.Marshal(fields)
}

func (s *GovernanceSpec) UnmarshalJSON(data []byte) error {
	type wire GovernanceSpec
	// Replace the old identity-based allowance when reading an existing world.
	// Work orders now carry their own rate and payment schedule.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, existed := fields["civic_stipend_per_cap"]; existed {
		delete(fields, "civic_stipend_per_cap")
		normalized, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		data = normalized
	}
	return unmarshalSpecNumbers(data, (*wire)(s), &s.explicitFields)
}

func (s GovernanceSpec) MarshalJSON() ([]byte, error) {
	type wire GovernanceSpec
	return marshalSpecNumbers(wire(s), s.explicitFields, false, s.ExplicitZero)
}

func (s *MarketCreditSpec) UnmarshalJSON(data []byte) error {
	type wire MarketCreditSpec
	if err := unmarshalSpecNumbers(data, (*wire)(s), &s.explicitFields); err != nil {
		return err
	}
	if s.MaxTenantSharePermille > 1000 {
		return fmt.Errorf("max_tenant_share_permille exceeds the whole share (1000)")
	}
	return nil
}

func (s MarketCreditSpec) MarshalJSON() ([]byte, error) {
	type wire MarketCreditSpec
	return marshalSpecNumbers(wire(s), s.explicitFields, false, s.ExplicitZero)
}

func (s *RelationEvoSpec) UnmarshalJSON(data []byte) error {
	type wire RelationEvoSpec
	return unmarshalSpecNumbers(data, (*wire)(s), &s.explicitFields)
}

func (s RelationEvoSpec) MarshalJSON() ([]byte, error) {
	type wire RelationEvoSpec
	return marshalSpecNumbers(wire(s), s.explicitFields, false, s.ExplicitZero)
}

func (s *JusticeRevoltSpec) UnmarshalJSON(data []byte) error {
	type wire JusticeRevoltSpec
	return unmarshalSpecNumbers(data, (*wire)(s), &s.explicitFields)
}

func (s JusticeRevoltSpec) MarshalJSON() ([]byte, error) {
	type wire JusticeRevoltSpec
	return marshalSpecNumbers(wire(s), s.explicitFields, false, s.ExplicitZero)
}

func (s *SpotlightRumorSpec) UnmarshalJSON(data []byte) error {
	type wire SpotlightRumorSpec
	return unmarshalSpecNumbers(data, (*wire)(s), &s.explicitFields)
}

func (s SpotlightRumorSpec) MarshalJSON() ([]byte, error) {
	type wire SpotlightRumorSpec
	return marshalSpecNumbers(wire(s), s.explicitFields, true, s.ExplicitZero)
}

func (s *LifecycleSpec) UnmarshalJSON(data []byte) error {
	type wire LifecycleSpec
	defaults := DefaultLifecycleSpec()
	decoded := wire{
		ChildMaxAge: defaults.ChildMaxAge, ElderMinAge: defaults.ElderMinAge,
		MaxExpectancy: defaults.MaxExpectancy, FertilityMinFoodPerCapita: defaults.FertilityMinFoodPerCapita,
		AnnualBirthPermille: defaults.AnnualBirthPermille, ElderAnnualDeathPermille: defaults.ElderAnnualDeathPermille,
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*s = LifecycleSpec(decoded)
	s.configured = true
	return nil
}
