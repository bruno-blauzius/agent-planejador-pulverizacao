package domain_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

var soilNow = time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)

func soilCalculator(t *testing.T) domain.SoilStateCalculator {
	t.Helper()
	calc, err := domain.NewSoilStateCalculator(domain.DefaultSoilTrafficabilityRules())
	if err != nil {
		t.Fatalf("NewSoilStateCalculator: %v", err)
	}
	return calc
}

func rain(source domain.RainSource, last48h, last72h float64) *domain.RainAccumulation {
	return &domain.RainAccumulation{Last48hMm: last48h, Last72hMm: last72h, Last24hMm: last48h, Last7dMm: last72h, Source: source, ObservedAt: soilNow}
}

func days(start time.Time, mm ...float64) []domain.DailyRain {
	forecast := make([]domain.DailyRain, len(mm))
	for i, v := range mm {
		forecast[i] = domain.DailyRain{Date: start.AddDate(0, 0, i+1), Mm: v}
	}
	return forecast
}

func fit(method domain.SprayMethod, status domain.MethodFitStatus, areaAbove *float64) domain.MethodFit {
	return domain.MethodFit{Method: method, Status: status, LimitSource: domain.LimitSourceReference, AreaAboveLimitPct: areaAbove}
}

func pct(v float64) *float64 { return &v }

func TestSoilStateCalculatorAssess(t *testing.T) {
	calc := soilCalculator(t)
	tests := []struct {
		name       string
		conditions domain.SoilConditions
		wantState  domain.SoilState
		wantBasis  domain.SoilBasis
	}{
		{
			name:       "case 1: clay, 24 mm in 48 h and 55 mm in 72 h is waterlogged",
			conditions: domain.SoilConditions{Texture: domain.TextureClay, Rain: rain(domain.RainFromGauge, 24, 55)},
			wantState:  domain.SoilWaterlogged, wantBasis: domain.BasisMeasuredRain,
		},
		{
			name: "case 2: gauge says 4 mm while the model says it is soaked; the gauge wins",
			conditions: domain.SoilConditions{
				Texture:       domain.TextureClay,
				Rain:          rain(domain.RainFromGauge, 4, 4),
				ModelMoisture: &domain.ModelSoilMoisture{VolumetricM3M3: 0.41, FieldCapacityM3M3: 0.36},
			},
			wantState: domain.SoilIdeal, wantBasis: domain.BasisMeasuredRain,
		},
		{
			name:       "case 3: sandy, 18 mm in 48 h is moist",
			conditions: domain.SoilConditions{Texture: domain.TextureSandy, Rain: rain(domain.RainFromStation, 18, 18)},
			wantState:  domain.SoilMoist, wantBasis: domain.BasisMeasuredRain,
		},
		{
			name: "case 6: inspection 10 h old replaces a waterlogged calculation",
			conditions: domain.SoilConditions{
				Texture:    domain.TextureClay,
				Rain:       rain(domain.RainFromGauge, 24, 55),
				Inspection: &domain.SoilInspection{State: domain.SoilIdeal, InspectedAt: soilNow.Add(-10 * time.Hour)},
			},
			wantState: domain.SoilIdeal, wantBasis: domain.BasisInspection,
		},
		{
			name: "case 6: inspection 13 h old is ignored",
			conditions: domain.SoilConditions{
				Texture:    domain.TextureClay,
				Rain:       rain(domain.RainFromGauge, 24, 55),
				Inspection: &domain.SoilInspection{State: domain.SoilIdeal, InspectedAt: soilNow.Add(-13 * time.Hour)},
			},
			wantState: domain.SoilWaterlogged, wantBasis: domain.BasisMeasuredRain,
		},
		{
			name: "inspection exactly 12 h old has expired",
			conditions: domain.SoilConditions{
				Texture:    domain.TextureClay,
				Rain:       rain(domain.RainFromGauge, 24, 55),
				Inspection: &domain.SoilInspection{State: domain.SoilIdeal, InspectedAt: soilNow.Add(-12 * time.Hour)},
			},
			wantState: domain.SoilWaterlogged, wantBasis: domain.BasisMeasuredRain,
		},
		{
			name:       "case 10: no gauge and no model data is unknown",
			conditions: domain.SoilConditions{Texture: domain.TextureMedium},
			wantState:  domain.SoilUnknown, wantBasis: domain.BasisNone,
		},
		{
			name: "model only: the worse of rain and moisture wins",
			conditions: domain.SoilConditions{
				Texture:       domain.TextureClay,
				Rain:          rain(domain.RainFromModel, 4, 4),
				ModelMoisture: &domain.ModelSoilMoisture{VolumetricM3M3: 0.41, FieldCapacityM3M3: 0.36},
			},
			wantState: domain.SoilWaterlogged, wantBasis: domain.BasisModel,
		},
		{
			name: "model only: drier moisture does not improve wet rain",
			conditions: domain.SoilConditions{
				Texture:       domain.TextureClay,
				Rain:          rain(domain.RainFromModel, 12, 12),
				ModelMoisture: &domain.ModelSoilMoisture{VolumetricM3M3: 0.10, FieldCapacityM3M3: 0.36},
			},
			wantState: domain.SoilMoist, wantBasis: domain.BasisModel,
		},
		{
			name:       "72 h rule: ideal 48 h rain with a wet third day worsens one state",
			conditions: domain.SoilConditions{Texture: domain.TextureClay, Rain: rain(domain.RainFromGauge, 6, 13)},
			wantState:  domain.SoilMoist, wantBasis: domain.BasisMeasuredRain,
		},
		{
			name:       "72 h rule: exactly 1.5 times the band limit does not worsen",
			conditions: domain.SoilConditions{Texture: domain.TextureClay, Rain: rain(domain.RainFromGauge, 6, 12)},
			wantState:  domain.SoilIdeal, wantBasis: domain.BasisMeasuredRain,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calc.Assess(soilNow, tt.conditions)
			if err != nil {
				t.Fatalf("Assess: %v", err)
			}
			if got.State != tt.wantState || got.Basis != tt.wantBasis {
				t.Errorf("got %s/%s, want %s/%s", got.State, got.Basis, tt.wantState, tt.wantBasis)
			}
		})
	}
}

func TestSoilStateBandBoundaries(t *testing.T) {
	calc := soilCalculator(t)
	tests := []struct {
		last48h float64
		want    domain.SoilState
	}{
		{0, domain.SoilDry},
		{1.9, domain.SoilDry},
		{2, domain.SoilIdeal},
		{8, domain.SoilIdeal},
		{8.1, domain.SoilMoist},
		{15, domain.SoilMoist},
		{15.1, domain.SoilWaterlogged},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("clay %v mm", tt.last48h), func(t *testing.T) {
			got, err := calc.Assess(soilNow, domain.SoilConditions{Texture: domain.TextureClay, Rain: rain(domain.RainFromGauge, tt.last48h, tt.last48h)})
			if err != nil {
				t.Fatalf("Assess: %v", err)
			}
			if got.State != tt.want {
				t.Errorf("clay %v mm: got %s, want %s", tt.last48h, got.State, tt.want)
			}
		})
	}
}

func TestSoilStateCalculatorRejectsUnknownTexture(t *testing.T) {
	calc := soilCalculator(t)
	_, err := calc.Assess(soilNow, domain.SoilConditions{Texture: "loam"})
	var validation *domain.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("got %v, want a validation error", err)
	}
}

func TestSoilTrafficabilityRulesValidate(t *testing.T) {
	broken := func(mutate func(*domain.SoilTrafficabilityRules)) domain.SoilTrafficabilityRules {
		rules := domain.DefaultSoilTrafficabilityRules()
		mutate(&rules)
		return rules
	}
	tests := []struct {
		name  string
		rules domain.SoilTrafficabilityRules
	}{
		{"missing texture", broken(func(r *domain.SoilTrafficabilityRules) { delete(r.Thresholds, domain.TextureClay) })},
		{"rain limits not increasing", broken(func(r *domain.SoilTrafficabilityRules) {
			clay := r.Thresholds[domain.TextureClay]
			clay.Rain48hUpperMm = [3]float64{2, 15, 8}
			r.Thresholds[domain.TextureClay] = clay
		})},
		{"zero drying rate", broken(func(r *domain.SoilTrafficabilityRules) {
			sandy := r.Thresholds[domain.TextureSandy]
			sandy.DryingRateMmPerDay = 0
			r.Thresholds[domain.TextureSandy] = sandy
		})},
		{"72 h factor below 1", broken(func(r *domain.SoilTrafficabilityRules) {
			medium := r.Thresholds[domain.TextureMedium]
			medium.Rain72hFactor = 0.5
			r.Thresholds[domain.TextureMedium] = medium
		})},
		{"no inspection validity", broken(func(r *domain.SoilTrafficabilityRules) { r.InspectionValidity = 0 })},
		{"zero field capacity", broken(func(r *domain.SoilTrafficabilityRules) {
			clay := r.Thresholds[domain.TextureClay]
			clay.FieldCapacityM3M3 = 0
			r.Thresholds[domain.TextureClay] = clay
		})},
		{"field capacity above 1", broken(func(r *domain.SoilTrafficabilityRules) {
			sandy := r.Thresholds[domain.TextureSandy]
			sandy.FieldCapacityM3M3 = 1.2
			r.Thresholds[domain.TextureSandy] = sandy
		})},
	}
	if err := domain.DefaultSoilTrafficabilityRules().Validate(); err != nil {
		t.Fatalf("defaults must be valid: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.rules.Validate(); !errors.Is(err, domain.ErrInvalidSoilRules) {
				t.Errorf("got %v, want ErrInvalidSoilRules", err)
			}
		})
	}
}

func TestProjectSoilStates(t *testing.T) {
	calc := soilCalculator(t)
	tests := []struct {
		name     string
		rain     *domain.RainAccumulation
		forecast []float64
		want     []domain.SoilState
	}{
		{
			name:     "case 4: waterlogged clay with three dry days reaches ideal on the third",
			rain:     rain(domain.RainFromGauge, 24, 55),
			forecast: []float64{0, 0, 0},
			want:     []domain.SoilState{domain.SoilWaterlogged, domain.SoilMoist, domain.SoilIdeal},
		},
		{
			name:     "doc example: 2 mm tomorrow, moist on day 2, ideal on day 3, 28 mm on day 5 blocks again",
			rain:     rain(domain.RainFromGauge, 24, 55),
			forecast: []float64{2, 0, 0, 0, 28},
			want:     []domain.SoilState{domain.SoilWaterlogged, domain.SoilMoist, domain.SoilIdeal, domain.SoilIdeal, domain.SoilWaterlogged},
		},
		{
			name:     "case 5: 28 mm before the release leaves no release within 5 days",
			rain:     rain(domain.RainFromGauge, 24, 55),
			forecast: []float64{28, 0, 0, 0, 0},
			want:     []domain.SoilState{domain.SoilWaterlogged, domain.SoilWaterlogged, domain.SoilWaterlogged, domain.SoilWaterlogged, domain.SoilWaterlogged},
		},
		{
			name:     "accumulation never goes below zero",
			rain:     rain(domain.RainFromGauge, 0, 0),
			forecast: []float64{0, 3},
			want:     []domain.SoilState{domain.SoilDry, domain.SoilIdeal},
		},
		{
			name:     "without rain every day is unknown",
			forecast: []float64{0, 0},
			want:     []domain.SoilState{domain.SoilUnknown, domain.SoilUnknown},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := calc.ProjectSoilStates(domain.TextureClay, tt.rain, days(soilNow, tt.forecast...))
			if err != nil {
				t.Fatalf("ProjectSoilStates: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// The matrix of the crossing document for a heavy ground method (airblast).
func TestEvaluateMethodEligibilityHeavyMatrix(t *testing.T) {
	soils := []domain.SoilState{domain.SoilDry, domain.SoilIdeal, domain.SoilMoist, domain.SoilWaterlogged}
	matrix := map[domain.MethodFitStatus][4]domain.EligibilityVerdict{
		domain.FitCompatible: {domain.VerdictApt, domain.VerdictApt, domain.VerdictAttention, domain.VerdictUnfit},
		domain.FitRestricted: {domain.VerdictAttention, domain.VerdictAttention, domain.VerdictUnfit, domain.VerdictUnfit},
		domain.FitUnsuitable: {domain.VerdictUnfit, domain.VerdictUnfit, domain.VerdictUnfit, domain.VerdictUnfit},
	}
	for status, row := range matrix {
		for i, soil := range soils {
			t.Run(string(status)+"/"+string(soil), func(t *testing.T) {
				f := fit(domain.MethodAirblast, status, nil)
				got := domain.EvaluateMethodEligibility(domain.MethodAirblast, &f, soil)
				if got.Verdict != row[i] {
					t.Errorf("got %s, want %s (reasons %v)", got.Verdict, row[i], got.Reasons)
				}
			})
		}
	}
}

func TestEvaluateMethodEligibilityCases(t *testing.T) {
	tests := []struct {
		name        string
		method      domain.SprayMethod
		fit         *domain.MethodFit
		soil        domain.SoilState
		wantVerdict domain.EligibilityVerdict
		wantReasons []domain.EligibilityReason
	}{
		{
			name: "case 1: waterlogged makes airblast unfit", method: domain.MethodAirblast,
			fit: ptrFit(fit(domain.MethodAirblast, domain.FitCompatible, pct(2))), soil: domain.SoilWaterlogged,
			wantVerdict: domain.VerdictUnfit, wantReasons: []domain.EligibilityReason{domain.ReasonSoilWaterlogged},
		},
		{
			name: "case 1: waterlogged makes self-propelled unfit", method: domain.MethodSelfPropelled,
			fit: ptrFit(fit(domain.MethodSelfPropelled, domain.FitCompatible, pct(2))), soil: domain.SoilWaterlogged,
			wantVerdict: domain.VerdictUnfit, wantReasons: []domain.EligibilityReason{domain.ReasonSoilWaterlogged},
		},
		{
			name: "case 1: drone stays apt on waterlogged soil", method: domain.MethodDrone,
			fit: ptrFit(fit(domain.MethodDrone, domain.FitCompatible, nil)), soil: domain.SoilWaterlogged,
			wantVerdict: domain.VerdictApt, wantReasons: []domain.EligibilityReason{},
		},
		{
			name: "case 3: knapsack is apt on moist soil", method: domain.MethodKnapsack,
			fit: ptrFit(fit(domain.MethodKnapsack, domain.FitCompatible, nil)), soil: domain.SoilMoist,
			wantVerdict: domain.VerdictApt, wantReasons: []domain.EligibilityReason{},
		},
		{
			name: "case 3: heavy methods need attention on moist soil", method: domain.MethodTractorBoom,
			fit: ptrFit(fit(domain.MethodTractorBoom, domain.FitCompatible, pct(0))), soil: domain.SoilMoist,
			wantVerdict: domain.VerdictAttention, wantReasons: []domain.EligibilityReason{domain.ReasonSoilMoist},
		},
		{
			name: "knapsack on waterlogged soil only needs attention", method: domain.MethodKnapsack,
			fit: ptrFit(fit(domain.MethodKnapsack, domain.FitCompatible, nil)), soil: domain.SoilWaterlogged,
			wantVerdict: domain.VerdictAttention, wantReasons: []domain.EligibilityReason{domain.ReasonSoilWaterlogged},
		},
		{
			name: "case 7: drone on waterlogged mountainous field is apt", method: domain.MethodDrone,
			fit: ptrFit(fit(domain.MethodDrone, domain.FitCompatible, nil)), soil: domain.SoilWaterlogged,
			wantVerdict: domain.VerdictApt, wantReasons: []domain.EligibilityReason{},
		},
		{
			name: "case 8: restricted slope and ideal soil is attention, with the area share", method: domain.MethodAirblast,
			fit: ptrFit(fit(domain.MethodAirblast, domain.FitRestricted, pct(38))), soil: domain.SoilIdeal,
			wantVerdict: domain.VerdictAttention, wantReasons: []domain.EligibilityReason{domain.ReasonSlopeRestricted},
		},
		{
			name: "restricted slope on moist soil is unfit for a heavy machine", method: domain.MethodAirblast,
			fit: ptrFit(fit(domain.MethodAirblast, domain.FitRestricted, pct(38))), soil: domain.SoilMoist,
			wantVerdict: domain.VerdictUnfit,
			wantReasons: []domain.EligibilityReason{domain.ReasonSlopeRestricted, domain.ReasonSoilMoist, domain.ReasonSlopeRestrictedOnWetSoil},
		},
		{
			name: "case 10: unknown soil keeps ground methods in attention", method: domain.MethodTractorBoom,
			fit: ptrFit(fit(domain.MethodTractorBoom, domain.FitCompatible, pct(0))), soil: domain.SoilUnknown,
			wantVerdict: domain.VerdictAttention, wantReasons: []domain.EligibilityReason{domain.ReasonSoilUnknown},
		},
		{
			name: "case 10: unknown soil keeps knapsack in attention, never apt", method: domain.MethodKnapsack,
			fit: ptrFit(fit(domain.MethodKnapsack, domain.FitCompatible, nil)), soil: domain.SoilUnknown,
			wantVerdict: domain.VerdictAttention, wantReasons: []domain.EligibilityReason{domain.ReasonSoilUnknown},
		},
		{
			name: "relief not calculated keeps heavy methods in attention", method: domain.MethodSelfPropelled,
			soil: domain.SoilIdeal, wantVerdict: domain.VerdictAttention,
			wantReasons: []domain.EligibilityReason{domain.ReasonTerrainUnknown},
		},
		{
			name: "unsuitable slope is unfit even with dry soil", method: domain.MethodSelfPropelled,
			fit: ptrFit(fit(domain.MethodSelfPropelled, domain.FitUnsuitable, pct(70))), soil: domain.SoilDry,
			wantVerdict: domain.VerdictUnfit, wantReasons: []domain.EligibilityReason{domain.ReasonSlopeUnsuitable},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.EvaluateMethodEligibility(tt.method, tt.fit, tt.soil)
			if got.Verdict != tt.wantVerdict {
				t.Errorf("verdict: got %s, want %s", got.Verdict, tt.wantVerdict)
			}
			if !slices.Equal(got.Reasons, tt.wantReasons) {
				t.Errorf("reasons: got %v, want %v", got.Reasons, tt.wantReasons)
			}
		})
	}
}

func TestEvaluateMethodEligibilityCarriesAreaShare(t *testing.T) {
	f := fit(domain.MethodAirblast, domain.FitRestricted, pct(38))
	got := domain.EvaluateMethodEligibility(domain.MethodAirblast, &f, domain.SoilIdeal)
	if got.AreaAboveLimitPct == nil || *got.AreaAboveLimitPct != 38 {
		t.Errorf("area above limit: got %v, want 38", got.AreaAboveLimitPct)
	}
}

func TestPlanTrafficability(t *testing.T) {
	calc := soilCalculator(t)
	today := time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC)
	accumulation := rain(domain.RainFromGauge, 24, 55)
	forecast := days(today, 2, 0, 0, 0, 28)
	projected, err := calc.ProjectSoilStates(domain.TextureClay, accumulation, forecast)
	if err != nil {
		t.Fatalf("ProjectSoilStates: %v", err)
	}
	dates := make([]time.Time, len(forecast))
	for i, day := range forecast {
		dates[i] = day.Date
	}
	fits := []domain.MethodFit{fit(domain.MethodAirblast, domain.FitCompatible, pct(2))}

	plan := domain.PlanTrafficability(today, domain.SoilWaterlogged, dates, projected, fits)

	if len(plan.Days) != 6 {
		t.Fatalf("days: got %d, want 6 (today and 5 forecast days)", len(plan.Days))
	}
	t.Run("today the airblast is unfit and the drone is apt", func(t *testing.T) {
		verdicts := verdictsOf(plan.Days[0])
		if verdicts[domain.MethodAirblast] != domain.VerdictUnfit || verdicts[domain.MethodDrone] != domain.VerdictApt {
			t.Errorf("got %v", verdicts)
		}
	})
	t.Run("on day 3 the airblast is apt again", func(t *testing.T) {
		if got := verdictsOf(plan.Days[3])[domain.MethodAirblast]; got != domain.VerdictApt {
			t.Errorf("got %s, want apt", got)
		}
	})
	t.Run("heavy methods are released on day 3, the knapsack on day 2", func(t *testing.T) {
		releases := map[domain.SprayMethod]*time.Time{}
		for _, release := range plan.Releases {
			releases[release.Method] = release.Date
		}
		day3, day2 := today.AddDate(0, 0, 3), today.AddDate(0, 0, 2)
		if got := releases[domain.MethodAirblast]; got == nil || !got.Equal(day3) {
			t.Errorf("airblast release: got %v, want %v", got, day3)
		}
		if got := releases[domain.MethodKnapsack]; got == nil || !got.Equal(day2) {
			t.Errorf("knapsack release: got %v, want %v", got, day2)
		}
		if _, ok := releases[domain.MethodDrone]; ok {
			t.Error("aerial methods have no soil release")
		}
	})
	t.Run("case 5: no release within the horizon", func(t *testing.T) {
		projected := []domain.SoilState{domain.SoilWaterlogged, domain.SoilWaterlogged}
		plan := domain.PlanTrafficability(today, domain.SoilWaterlogged, dates[:2], projected, fits)
		for _, release := range plan.Releases {
			if release.Date != nil {
				t.Errorf("%s: got release %v, want none", release.Method, release.Date)
			}
		}
	})
}

func ptrFit(f domain.MethodFit) *domain.MethodFit { return &f }

func verdictsOf(day domain.TrafficabilityDay) map[domain.SprayMethod]domain.EligibilityVerdict {
	verdicts := make(map[domain.SprayMethod]domain.EligibilityVerdict, len(day.Methods))
	for _, m := range day.Methods {
		verdicts[m.Method] = m.Verdict
	}
	return verdicts
}

func TestSoilStateOrdering(t *testing.T) {
	tests := []struct {
		a, b, want domain.SoilState
	}{
		{domain.SoilDry, domain.SoilWaterlogged, domain.SoilWaterlogged},
		{domain.SoilMoist, domain.SoilIdeal, domain.SoilMoist},
		{domain.SoilUnknown, domain.SoilIdeal, domain.SoilIdeal},
		{domain.SoilIdeal, domain.SoilUnknown, domain.SoilIdeal},
	}
	for _, tt := range tests {
		t.Run(string(tt.a)+"+"+string(tt.b), func(t *testing.T) {
			if got := domain.WorstSoilState(tt.a, tt.b); got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
	if domain.SoilUnknown.Meets(domain.SoilMoist) {
		t.Error("unknown must never meet a requirement")
	}
	if !domain.SoilDry.Meets(domain.SoilIdeal) {
		t.Error("dry meets ideal")
	}
}
