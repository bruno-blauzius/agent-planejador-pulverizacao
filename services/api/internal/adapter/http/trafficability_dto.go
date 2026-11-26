package httpadapter

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Bodies of the Trafficability tag. They only copy what the use cases
// computed; no soil rule lives here.

type rainGaugeBody struct {
	ID        string        `json:"id"`
	FarmID    string        `json:"farm_id"`
	Name      string        `json:"name"`
	Location  *geoPointBody `json:"location"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Version   int           `json:"version"`
}

func rainGaugeBodyOf(gauge domain.RainGauge) rainGaugeBody {
	return rainGaugeBody{
		ID: string(gauge.ID), FarmID: string(gauge.FarmID), Name: gauge.Name, Location: pointOf(gauge.Location),
		CreatedAt: gauge.CreatedAt, UpdatedAt: gauge.UpdatedAt, Version: gauge.Version,
	}
}

type rainReadingBody struct {
	ID         string        `json:"id"`
	GaugeID    string        `json:"rain_gauge_id"`
	RainMm     float64       `json:"rain_mm"`
	ObservedAt time.Time     `json:"observed_at"`
	RecordedBy personRefBody `json:"recorded_by"`
	RecordedAt time.Time     `json:"recorded_at"`
}

func rainReadingBodyOf(stored usecase.StoredRainReading) rainReadingBody {
	reading := stored.Reading
	return rainReadingBody{
		ID: string(reading.ID), GaugeID: string(reading.GaugeID), RainMm: reading.RainMm, ObservedAt: reading.ObservedAt,
		RecordedBy: personRefBody{ID: string(reading.RecordedBy), Name: stored.RecordedByName}, RecordedAt: reading.RecordedAt,
	}
}

type soilInspectionBody struct {
	ID          string        `json:"id"`
	FieldID     string        `json:"field_id"`
	FarmID      string        `json:"farm_id"`
	State       string        `json:"state"`
	InspectedAt time.Time     `json:"inspected_at"`
	ValidUntil  time.Time     `json:"valid_until"`
	InspectedBy personRefBody `json:"inspected_by"`
	Note        *string       `json:"note"`
}

func soilInspectionBodyOf(view usecase.SoilInspectionView) soilInspectionBody {
	inspection := view.Inspection
	return soilInspectionBody{
		ID: string(inspection.ID), FieldID: string(inspection.FieldID), FarmID: string(inspection.FarmID),
		State: string(inspection.State), InspectedAt: inspection.InspectedAt, ValidUntil: view.ValidUntil,
		InspectedBy: personRefBody{ID: string(inspection.InspectedBy), Name: view.InspectedByName}, Note: inspection.Note,
	}
}

type soilProfileBody struct {
	FieldID     string         `json:"field_id"`
	FarmID      string         `json:"farm_id"`
	SoilTexture *string        `json:"soil_texture"`
	RainGauge   *personRefBody `json:"rain_gauge"`
}

func soilProfileBodyOf(view usecase.SoilProfileView) soilProfileBody {
	body := soilProfileBody{FieldID: string(view.FieldID), FarmID: string(view.FarmID)}
	if view.Texture != nil {
		texture := string(*view.Texture)
		body.SoilTexture = &texture
	}
	if view.Gauge != nil {
		body.RainGauge = &personRefBody{ID: string(view.Gauge.ID), Name: view.Gauge.Name}
	}
	return body
}

// ---- trafficability ------------------------------------------------------------------

type rainAccumulationBody struct {
	Last24hMm  float64   `json:"last_24h_mm"`
	Last48hMm  float64   `json:"last_48h_mm"`
	Last72hMm  float64   `json:"last_72h_mm"`
	Last7dMm   float64   `json:"last_7d_mm"`
	Source     string    `json:"source"`
	SourceName *string   `json:"source_name"`
	ObservedAt time.Time `json:"observed_at"`
}

type modelSoilMoistureBody struct {
	VolumetricM3M3    float64 `json:"volumetric_m3m3"`
	FieldCapacityM3M3 float64 `json:"field_capacity_m3m3"`
	FieldCapacityPct  float64 `json:"field_capacity_pct"`
}

type soilInspectionViewBody struct {
	State       string        `json:"state"`
	InspectedAt time.Time     `json:"inspected_at"`
	ValidUntil  time.Time     `json:"valid_until"`
	InspectedBy personRefBody `json:"inspected_by"`
}

type soilStatusBody struct {
	State            string                  `json:"state"`
	Basis            string                  `json:"basis"`
	Texture          *string                 `json:"texture"`
	ThresholdsSource string                  `json:"thresholds_source"`
	Rain             *rainAccumulationBody   `json:"rain"`
	ModelMoisture    *modelSoilMoistureBody  `json:"model_moisture"`
	Inspection       *soilInspectionViewBody `json:"inspection"`
}

type dailyRainBody struct {
	Date   string  `json:"date"`
	RainMm float64 `json:"rain_mm"`
	Kind   string  `json:"kind"`
	Source string  `json:"source"`
}

type methodEligibilityBody struct {
	Method            string   `json:"method"`
	Verdict           string   `json:"verdict"`
	Reasons           []string `json:"reasons"`
	AreaAboveLimitPct *float64 `json:"area_above_limit_pct"`
}

type trafficabilityDayBody struct {
	Date      string                  `json:"date"`
	SoilState string                  `json:"soil_state"`
	Methods   []methodEligibilityBody `json:"methods"`
}

type methodReleaseBody struct {
	Method        string  `json:"method"`
	RequiredState string  `json:"required_state"`
	Date          *string `json:"date"`
}

type fieldTrafficabilityBody struct {
	FieldID    string                  `json:"field_id"`
	FarmID     string                  `json:"farm_id"`
	AsOf       time.Time               `json:"as_of"`
	Soil       soilStatusBody          `json:"soil"`
	RainDaily  []dailyRainBody         `json:"rain_daily"`
	Days       []trafficabilityDayBody `json:"days"`
	Releases   []methodReleaseBody     `json:"releases"`
	DataIssues []string                `json:"data_issues"`
}

func soilStatusBodyOf(soil usecase.SoilStatusView) soilStatusBody {
	body := soilStatusBody{State: string(soil.State), Basis: string(soil.Basis), ThresholdsSource: soil.ThresholdsSource}
	if soil.Texture != nil {
		texture := string(*soil.Texture)
		body.Texture = &texture
	}
	if rain := soil.Rain; rain != nil {
		body.Rain = &rainAccumulationBody{
			Last24hMm: rain.Last24hMm, Last48hMm: rain.Last48hMm, Last72hMm: rain.Last72hMm, Last7dMm: rain.Last7dMm,
			Source: string(rain.Source), SourceName: soil.RainSourceName, ObservedAt: rain.ObservedAt,
		}
	}
	if moisture := soil.ModelMoisture; moisture != nil {
		pct, _ := moisture.FieldCapacityPct()
		body.ModelMoisture = &modelSoilMoistureBody{
			VolumetricM3M3: moisture.VolumetricM3M3, FieldCapacityM3M3: moisture.FieldCapacityM3M3, FieldCapacityPct: pct,
		}
	}
	if view := soil.Inspection; view != nil {
		body.Inspection = &soilInspectionViewBody{
			State: string(view.Inspection.State), InspectedAt: view.Inspection.InspectedAt, ValidUntil: view.ValidUntil,
			InspectedBy: personRefBody{ID: string(view.Inspection.InspectedBy), Name: view.InspectedByName},
		}
	}
	return body
}

func fieldTrafficabilityOf(view usecase.FieldTrafficabilityView) fieldTrafficabilityBody {
	body := fieldTrafficabilityBody{
		FieldID: string(view.FieldID), FarmID: string(view.FarmID), AsOf: view.AsOf, Soil: soilStatusBodyOf(view.Soil),
		RainDaily: make([]dailyRainBody, 0, len(view.RainDaily)), Days: make([]trafficabilityDayBody, 0, len(view.Plan.Days)),
		Releases: make([]methodReleaseBody, 0, len(view.Plan.Releases)), DataIssues: make([]string, 0, len(view.DataIssues)),
	}
	for _, entry := range view.RainDaily {
		body.RainDaily = append(body.RainDaily, dailyRainBody{
			Date: entry.Date.Format(dateLayout), RainMm: entry.RainMm, Kind: string(entry.Kind), Source: string(entry.Source),
		})
	}
	for _, day := range view.Plan.Days {
		dayBody := trafficabilityDayBody{Date: day.Date.Format(dateLayout), SoilState: string(day.SoilState), Methods: make([]methodEligibilityBody, 0, len(day.Methods))}
		for _, method := range day.Methods {
			reasons := make([]string, 0, len(method.Reasons))
			for _, reason := range method.Reasons {
				reasons = append(reasons, string(reason))
			}
			dayBody.Methods = append(dayBody.Methods, methodEligibilityBody{
				Method: string(method.Method), Verdict: string(method.Verdict), Reasons: reasons, AreaAboveLimitPct: method.AreaAboveLimitPct,
			})
		}
		body.Days = append(body.Days, dayBody)
	}
	for _, release := range view.Plan.Releases {
		releaseBody := methodReleaseBody{Method: string(release.Method), RequiredState: string(release.RequiredState)}
		if release.Date != nil {
			date := release.Date.Format(dateLayout)
			releaseBody.Date = &date
		}
		body.Releases = append(body.Releases, releaseBody)
	}
	for _, issue := range view.DataIssues {
		body.DataIssues = append(body.DataIssues, string(issue))
	}
	return body
}

// ---- inputs --------------------------------------------------------------------------

// optionalJSON tells an absent key from an explicit null: UnmarshalJSON is
// only called when the key is present (null included).
type optionalJSON struct {
	Set bool
	Raw json.RawMessage
}

// UnmarshalJSON implements json.Unmarshaler.
func (o *optionalJSON) UnmarshalJSON(data []byte) error {
	o.Set = true
	o.Raw = append(o.Raw[:0], data...)
	return nil
}

// isNull reports whether the key was sent as null.
func (o optionalJSON) isNull() bool { return bytes.Equal(bytes.TrimSpace(o.Raw), []byte("null")) }

// decodeInto reads the value; a type mismatch is a validation error on field.
func (o optionalJSON) decodeInto(field string, target any) error {
	if err := json.Unmarshal(o.Raw, target); err != nil {
		return validationError("dados inválidos", fieldDetail{Field: field, Message: "tipo de valor inválido"})
	}
	return nil
}
