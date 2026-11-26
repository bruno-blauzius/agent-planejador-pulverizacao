package httpadapter

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// maxConditionsBodyBytes bounds the small JSON bodies of the Trafficability tag.
const maxConditionsBodyBytes = 16 << 10

// The use cases the handler consumes (declared here, where they are used).

// RainGaugeLister lists the gauges of a farm.
type RainGaugeLister interface {
	Execute(ctx context.Context, in usecase.ListRainGaugesInput) ([]domain.RainGauge, error)
}

// RainGaugeCreator registers a gauge.
type RainGaugeCreator interface {
	Execute(ctx context.Context, in usecase.CreateRainGaugeInput) (domain.RainGauge, error)
}

// RainGaugeUpdater changes a gauge.
type RainGaugeUpdater interface {
	Execute(ctx context.Context, in usecase.UpdateRainGaugeInput) (domain.RainGauge, error)
}

// RainReadingLister lists the readings of a gauge.
type RainReadingLister interface {
	Execute(ctx context.Context, in usecase.ListRainReadingsInput) ([]usecase.StoredRainReading, error)
}

// RainReadingRecorder records a reading.
type RainReadingRecorder interface {
	Execute(ctx context.Context, in usecase.RecordRainReadingInput) (usecase.StoredRainReading, error)
}

// SoilInspectionRecorder records a soil inspection.
type SoilInspectionRecorder interface {
	Execute(ctx context.Context, in usecase.RecordSoilInspectionInput) (usecase.SoilInspectionView, error)
}

// SoilProfileGetter reads the soil profile of a field.
type SoilProfileGetter interface {
	Execute(ctx context.Context, in usecase.GetSoilProfileInput) (usecase.SoilProfileView, error)
}

// SoilProfileUpdater replaces the soil profile of a field.
type SoilProfileUpdater interface {
	Execute(ctx context.Context, in usecase.UpdateSoilProfileInput) (usecase.SoilProfileView, error)
}

// FieldTrafficabilityGetter reads the trafficability of a field.
type FieldTrafficabilityGetter interface {
	Execute(ctx context.Context, in usecase.GetFieldTrafficabilityInput) (usecase.FieldTrafficabilityView, error)
}

// TrafficabilityDeps groups the use cases of TrafficabilityHandler.
type TrafficabilityDeps struct {
	ListGauges        RainGaugeLister
	CreateGauge       RainGaugeCreator
	UpdateGauge       RainGaugeUpdater
	ListReadings      RainReadingLister
	RecordReading     RainReadingRecorder
	RecordInspection  SoilInspectionRecorder
	GetSoilProfile    SoilProfileGetter
	UpdateSoilProfile SoilProfileUpdater
	GetTrafficability FieldTrafficabilityGetter
}

// TrafficabilityHandler serves the Trafficability tag of the contract (ADR 0022).
type TrafficabilityHandler struct {
	logger *slog.Logger
	deps   TrafficabilityDeps
}

// NewTrafficabilityHandler builds the handler.
func NewTrafficabilityHandler(logger *slog.Logger, deps TrafficabilityDeps) *TrafficabilityHandler {
	return &TrafficabilityHandler{logger: logger, deps: deps}
}

// Register attaches the routes (protected; capabilities are checked by the use cases).
func (h *TrafficabilityHandler) Register(routes Routes) {
	routes.Protected(http.MethodGet, "/farms/{farmId}/rain-gauges", serve(h.logger, h.listGauges))
	routes.Protected(http.MethodPost, "/farms/{farmId}/rain-gauges", serve(h.logger, h.createGauge))
	routes.Protected(http.MethodPatch, "/farms/{farmId}/rain-gauges/{gaugeId}", serve(h.logger, h.updateGauge))
	routes.Protected(http.MethodGet, "/farms/{farmId}/rain-gauges/{gaugeId}/readings", serve(h.logger, h.listReadings))
	routes.Protected(http.MethodPost, "/farms/{farmId}/rain-gauges/{gaugeId}/readings", serve(h.logger, h.recordReading))
	routes.Protected(http.MethodPost, "/farms/{farmId}/fields/{fieldId}/soil-inspections", serve(h.logger, h.recordInspection))
	routes.Protected(http.MethodGet, "/farms/{farmId}/fields/{fieldId}/soil-profile", serve(h.logger, h.getSoilProfile))
	routes.Protected(http.MethodPut, "/farms/{farmId}/fields/{fieldId}/soil-profile", serve(h.logger, h.updateSoilProfile))
	routes.Protected(http.MethodGet, "/farms/{farmId}/fields/{fieldId}/trafficability", serve(h.logger, h.getTrafficability))
}

func gaugePath(r *http.Request) (domain.FarmID, domain.RainGaugeID, error) {
	farmID, err := pathID(r, "farmId")
	if err != nil {
		return "", "", err
	}
	gaugeID, err := pathID(r, "gaugeId")
	if err != nil {
		return "", "", err
	}
	return domain.FarmID(farmID), domain.RainGaugeID(gaugeID), nil
}

// ---- rain gauges ---------------------------------------------------------------------

func (h *TrafficabilityHandler) listGauges(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, err := pathID(r, "farmId")
	if err != nil {
		return response{}, err
	}
	gauges, err := h.deps.ListGauges.Execute(r.Context(), usecase.ListRainGaugesInput{Actor: actor, FarmID: domain.FarmID(farmID)})
	if err != nil {
		return response{}, err
	}
	items := make([]rainGaugeBody, 0, len(gauges))
	for _, gauge := range gauges {
		items = append(items, rainGaugeBodyOf(gauge))
	}
	return okResponse(map[string]any{"items": items}), nil
}

// rainGaugeInputBody is the body of POST .../rain-gauges.
type rainGaugeInputBody struct {
	Name     string        `json:"name"`
	Location *geoPointBody `json:"location"`
}

func domainPointOf(body *geoPointBody) *domain.GeoPoint {
	if body == nil {
		return nil
	}
	return &domain.GeoPoint{Lat: body.Lat, Lng: body.Lng}
}

func (h *TrafficabilityHandler) createGauge(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, err := pathID(r, "farmId")
	if err != nil {
		return response{}, err
	}
	var body rainGaugeInputBody
	if err := decodeJSONWithin(w, r, &body, maxConditionsBodyBytes); err != nil {
		return response{}, err
	}
	gauge, err := h.deps.CreateGauge.Execute(r.Context(), usecase.CreateRainGaugeInput{
		Actor: actor, FarmID: domain.FarmID(farmID), Details: domain.RainGaugeDetails{Name: body.Name, Location: domainPointOf(body.Location)},
	})
	if err != nil {
		return response{}, err
	}
	return createdResponse(rainGaugeBodyOf(gauge)), nil
}

// rainGaugePatchBody is the body of PATCH .../rain-gauges/{gaugeId}: an absent
// key keeps the value; "location": null removes the location.
type rainGaugePatchBody struct {
	Name     optionalJSON `json:"name"`
	Location optionalJSON `json:"location"`
}

func (b rainGaugePatchBody) patch() (usecase.RainGaugePatch, error) {
	var patch usecase.RainGaugePatch
	if b.Name.Set {
		if b.Name.isNull() {
			return patch, validationError("dados inválidos", fieldDetail{Field: "name", Message: "não pode ser nulo"})
		}
		var name string
		if err := b.Name.decodeInto("name", &name); err != nil {
			return patch, err
		}
		patch.Name = &name
	}
	if b.Location.Set {
		patch.LocationSet = true
		if !b.Location.isNull() {
			var point geoPointBody
			if err := b.Location.decodeInto("location", &point); err != nil {
				return patch, err
			}
			patch.Location = domainPointOf(&point)
		}
	}
	return patch, nil
}

func (h *TrafficabilityHandler) updateGauge(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, gaugeID, err := gaugePath(r)
	if err != nil {
		return response{}, err
	}
	var body rainGaugePatchBody
	if err := decodeJSONWithin(w, r, &body, maxConditionsBodyBytes); err != nil {
		return response{}, err
	}
	patch, err := body.patch()
	if err != nil {
		return response{}, err
	}
	gauge, err := h.deps.UpdateGauge.Execute(r.Context(), usecase.UpdateRainGaugeInput{Actor: actor, FarmID: farmID, GaugeID: gaugeID, Patch: patch})
	if err != nil {
		return response{}, err
	}
	return okResponse(rainGaugeBodyOf(gauge)), nil
}

// ---- readings ------------------------------------------------------------------------

// instant reads an optional RFC 3339 query parameter.
func (q *query) instant(name string) time.Time {
	raw := strings.TrimSpace(q.values.Get(name))
	if raw == "" {
		return time.Time{}
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		q.problems.Add(name, "data e hora inválidas (use RFC 3339, ex.: 2026-11-26T07:00:00-03:00)")
		return time.Time{}
	}
	return value
}

func (h *TrafficabilityHandler) listReadings(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, gaugeID, err := gaugePath(r)
	if err != nil {
		return response{}, err
	}
	q := newQuery(r)
	after, until := q.instant("from"), q.instant("to")
	if err := q.err(); err != nil {
		return response{}, err
	}
	readings, err := h.deps.ListReadings.Execute(r.Context(), usecase.ListRainReadingsInput{
		Actor: actor, FarmID: farmID, GaugeID: gaugeID, After: after, Until: until,
	})
	if err != nil {
		return response{}, err
	}
	items := make([]rainReadingBody, 0, len(readings))
	for _, reading := range readings {
		items = append(items, rainReadingBodyOf(reading))
	}
	return okResponse(map[string]any{"items": items}), nil
}

// rainReadingInputBody is the body of POST .../readings. The pointers tell a
// missing value from zero (0 mm is a valid reading).
type rainReadingInputBody struct {
	RainMm     *float64   `json:"rain_mm"`
	ObservedAt *time.Time `json:"observed_at"`
}

func (h *TrafficabilityHandler) recordReading(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, gaugeID, err := gaugePath(r)
	if err != nil {
		return response{}, err
	}
	var body rainReadingInputBody
	if err := decodeJSONWithin(w, r, &body, maxConditionsBodyBytes); err != nil {
		return response{}, err
	}
	var missing domain.Violations
	if body.RainMm == nil {
		missing.Add("rain_mm", "é obrigatório")
	}
	if body.ObservedAt == nil {
		missing.Add("observed_at", "é obrigatório")
	}
	if err := missing.Err(); err != nil {
		return response{}, err
	}
	stored, err := h.deps.RecordReading.Execute(r.Context(), usecase.RecordRainReadingInput{
		Actor: actor, FarmID: farmID, GaugeID: gaugeID, RainMm: *body.RainMm, ObservedAt: *body.ObservedAt,
	})
	if err != nil {
		return response{}, err
	}
	return createdResponse(rainReadingBodyOf(stored)), nil
}

// ---- soil inspections and profile ----------------------------------------------------

// soilInspectionInputBody is the body of POST .../soil-inspections.
type soilInspectionInputBody struct {
	State       string     `json:"state"`
	InspectedAt *time.Time `json:"inspected_at"`
	Note        *string    `json:"note"`
}

func (h *TrafficabilityHandler) recordInspection(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, fieldID, err := fieldPath(r)
	if err != nil {
		return response{}, err
	}
	var body soilInspectionInputBody
	if err := decodeJSONWithin(w, r, &body, maxConditionsBodyBytes); err != nil {
		return response{}, err
	}
	details := domain.SoilInspectionDetails{State: domain.SoilState(body.State), Note: body.Note}
	if body.InspectedAt != nil {
		details.InspectedAt = *body.InspectedAt
	}
	view, err := h.deps.RecordInspection.Execute(r.Context(), usecase.RecordSoilInspectionInput{
		Actor: actor, FarmID: farmID, FieldID: fieldID, Details: details,
	})
	if err != nil {
		return response{}, err
	}
	return createdResponse(soilInspectionBodyOf(view)), nil
}

func (h *TrafficabilityHandler) getSoilProfile(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, fieldID, err := fieldPath(r)
	if err != nil {
		return response{}, err
	}
	view, err := h.deps.GetSoilProfile.Execute(r.Context(), usecase.GetSoilProfileInput{Actor: actor, FarmID: farmID, FieldID: fieldID})
	if err != nil {
		return response{}, err
	}
	return okResponse(soilProfileBodyOf(view)), nil
}

// soilProfileInputBody is the body of PUT .../soil-profile. Both keys are
// required (null clears), so a client cannot clear a value by forgetting it.
type soilProfileInputBody struct {
	SoilTexture optionalJSON `json:"soil_texture"`
	RainGaugeID optionalJSON `json:"rain_gauge_id"`
}

func (b soilProfileInputBody) values() (*domain.SoilTexture, *domain.RainGaugeID, error) {
	var problems domain.Violations
	if !b.SoilTexture.Set {
		problems.Add("soil_texture", "é obrigatório (use null para limpar)")
	}
	if !b.RainGaugeID.Set {
		problems.Add("rain_gauge_id", "é obrigatório (use null para limpar)")
	}
	if err := problems.Err(); err != nil {
		return nil, nil, err
	}
	var texture *domain.SoilTexture
	if !b.SoilTexture.isNull() {
		var value string
		if err := b.SoilTexture.decodeInto("soil_texture", &value); err != nil {
			return nil, nil, err
		}
		converted := domain.SoilTexture(value)
		texture = &converted
	}
	var gaugeID *domain.RainGaugeID
	if !b.RainGaugeID.isNull() {
		var value string
		if err := b.RainGaugeID.decodeInto("rain_gauge_id", &value); err != nil {
			return nil, nil, err
		}
		id, ok := canonicalUUID(value)
		if !ok {
			return nil, nil, validationError("dados inválidos", fieldDetail{Field: "rain_gauge_id", Message: "identificador inválido"})
		}
		converted := domain.RainGaugeID(id)
		gaugeID = &converted
	}
	return texture, gaugeID, nil
}

func (h *TrafficabilityHandler) updateSoilProfile(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, fieldID, err := fieldPath(r)
	if err != nil {
		return response{}, err
	}
	var body soilProfileInputBody
	if err := decodeJSONWithin(w, r, &body, maxConditionsBodyBytes); err != nil {
		return response{}, err
	}
	texture, gaugeID, err := body.values()
	if err != nil {
		return response{}, err
	}
	view, err := h.deps.UpdateSoilProfile.Execute(r.Context(), usecase.UpdateSoilProfileInput{
		Actor: actor, FarmID: farmID, FieldID: fieldID, Texture: texture, GaugeID: gaugeID,
	})
	if err != nil {
		return response{}, err
	}
	return okResponse(soilProfileBodyOf(view)), nil
}

// ---- trafficability ------------------------------------------------------------------

func (h *TrafficabilityHandler) getTrafficability(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, fieldID, err := fieldPath(r)
	if err != nil {
		return response{}, err
	}
	q := newQuery(r)
	days := q.positiveInt("days")
	if err := q.err(); err != nil {
		return response{}, err
	}
	view, err := h.deps.GetTrafficability.Execute(r.Context(), usecase.GetFieldTrafficabilityInput{
		Actor: actor, FarmID: farmID, FieldID: fieldID, Days: days,
	})
	if err != nil {
		return response{}, err
	}
	return okResponse(fieldTrafficabilityOf(view)), nil
}
