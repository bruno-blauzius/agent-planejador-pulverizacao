package httpadapter

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// maxTerrainBodyBytes bounds the small JSON bodies of the Terrain tag.
const maxTerrainBodyBytes = 64 << 10

// The use cases the handler consumes (declared here, where they are used).

// FarmTerrainGetter reads the relief of a farm.
type FarmTerrainGetter interface {
	Execute(ctx context.Context, in usecase.GetFarmTerrainInput) (usecase.FarmTerrainView, error)
}

// FarmTerrainCalculator schedules the relief calculation of a farm.
type FarmTerrainCalculator interface {
	Execute(ctx context.Context, in usecase.CalculateFarmTerrainInput) (usecase.FarmTerrainView, error)
}

// FieldTerrainGetter reads the relief of a field.
type FieldTerrainGetter interface {
	Execute(ctx context.Context, in usecase.GetFieldTerrainInput) (usecase.FieldTerrainView, error)
}

// FieldTerrainAdjuster stores the manual adjustment of a field.
type FieldTerrainAdjuster interface {
	Execute(ctx context.Context, in usecase.AdjustFieldTerrainInput) (usecase.FieldTerrainView, error)
}

// FieldTerrainAdjustmentRemover drops the manual adjustment of a field.
type FieldTerrainAdjustmentRemover interface {
	Execute(ctx context.Context, in usecase.RemoveFieldTerrainAdjustmentInput) (usecase.FieldTerrainView, error)
}

// TerrainDeps groups the use cases of TerrainHandler.
type TerrainDeps struct {
	GetFarm          FarmTerrainGetter
	Calculate        FarmTerrainCalculator
	GetField         FieldTerrainGetter
	Adjust           FieldTerrainAdjuster
	RemoveAdjustment FieldTerrainAdjustmentRemover
}

// TerrainHandler serves the Terrain tag of the contract.
type TerrainHandler struct {
	logger *slog.Logger
	deps   TerrainDeps
}

// NewTerrainHandler builds the handler.
func NewTerrainHandler(logger *slog.Logger, deps TerrainDeps) *TerrainHandler {
	return &TerrainHandler{logger: logger, deps: deps}
}

// Register attaches the routes (protected; registry.write is checked by the use cases).
func (h *TerrainHandler) Register(routes Routes) {
	routes.Protected(http.MethodGet, "/farms/{farmId}/terrain", serve(h.logger, h.getFarmTerrain))
	routes.Protected(http.MethodPost, "/farms/{farmId}/terrain/calculations", serve(h.logger, h.calculateFarmTerrain))
	routes.Protected(http.MethodGet, "/farms/{farmId}/fields/{fieldId}/terrain", serve(h.logger, h.getFieldTerrain))
	routes.Protected(http.MethodPut, "/farms/{farmId}/fields/{fieldId}/terrain/adjustment", serve(h.logger, h.adjustFieldTerrain))
	routes.Protected(http.MethodDelete, "/farms/{farmId}/fields/{fieldId}/terrain/adjustment", serve(h.logger, h.removeFieldTerrainAdjustment))
}

func (h *TerrainHandler) getFarmTerrain(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, err := pathID(r, "farmId")
	if err != nil {
		return response{}, err
	}
	view, err := h.deps.GetFarm.Execute(r.Context(), usecase.GetFarmTerrainInput{Actor: actor, FarmID: domain.FarmID(farmID)})
	if err != nil {
		return response{}, err
	}
	return okResponse(farmTerrainOf(view)), nil
}

// calculationRequestBody is the optional body of POST .../terrain/calculations.
type calculationRequestBody struct {
	Scope *string `json:"scope"`
}

// readCalculationScope returns the requested scope. An empty body or an absent
// scope leaves it empty, which the use case reads as "missing"; an unknown
// value is rejected by the use case with a 422 on scope.
func readCalculationScope(w http.ResponseWriter, r *http.Request) (usecase.TerrainRunScope, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTerrainBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return "", bodyFailure(err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	var body calculationRequestBody
	if err := decodeJSONWithin(w, r, &body, maxTerrainBodyBytes); err != nil {
		return "", err
	}
	if body.Scope == nil {
		return "", nil
	}
	return usecase.TerrainRunScope(*body.Scope), nil
}

func (h *TerrainHandler) calculateFarmTerrain(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, err := pathID(r, "farmId")
	if err != nil {
		return response{}, err
	}
	scope, err := readCalculationScope(w, r)
	if err != nil {
		return response{}, err
	}
	view, err := h.deps.Calculate.Execute(r.Context(), usecase.CalculateFarmTerrainInput{
		Actor: actor, FarmID: domain.FarmID(farmID), Scope: scope,
	})
	if err != nil {
		return response{}, err
	}
	return response{status: http.StatusAccepted, body: farmTerrainOf(view)}, nil
}

func fieldPath(r *http.Request) (domain.FarmID, domain.FieldID, error) {
	farmID, err := pathID(r, "farmId")
	if err != nil {
		return "", "", err
	}
	fieldID, err := pathID(r, "fieldId")
	if err != nil {
		return "", "", err
	}
	return domain.FarmID(farmID), domain.FieldID(fieldID), nil
}

func (h *TerrainHandler) getFieldTerrain(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, fieldID, err := fieldPath(r)
	if err != nil {
		return response{}, err
	}
	view, err := h.deps.GetField.Execute(r.Context(), usecase.GetFieldTerrainInput{Actor: actor, FarmID: farmID, FieldID: fieldID})
	if err != nil {
		return response{}, err
	}
	return okResponse(fieldTerrainOf(view)), nil
}

// terrainAdjustmentInputBody is the body of PUT .../terrain/adjustment. The
// numbers are pointers so that a missing one is told apart from zero.
type terrainAdjustmentInputBody struct {
	Origin        string   `json:"origin"`
	SlopeMeanPct  *float64 `json:"slope_mean_pct"`
	SlopeMaxPct   *float64 `json:"slope_max_pct"`
	Position      string   `json:"position"`
	Justification string   `json:"justification"`
}

func (b terrainAdjustmentInputBody) details() (domain.TerrainAdjustmentDetails, error) {
	var missing domain.Violations
	if b.SlopeMeanPct == nil {
		missing.Add("slope_mean_pct", "é obrigatório")
	}
	if b.SlopeMaxPct == nil {
		missing.Add("slope_max_pct", "é obrigatório")
	}
	if err := missing.Err(); err != nil {
		return domain.TerrainAdjustmentDetails{}, err
	}
	return domain.TerrainAdjustmentDetails{
		Origin:        domain.TerrainAdjustmentOrigin(b.Origin),
		SlopeMeanPct:  *b.SlopeMeanPct,
		SlopeMaxPct:   *b.SlopeMaxPct,
		Position:      domain.TerrainPosition(b.Position),
		Justification: b.Justification,
	}, nil
}

func (h *TerrainHandler) adjustFieldTerrain(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, fieldID, err := fieldPath(r)
	if err != nil {
		return response{}, err
	}
	var body terrainAdjustmentInputBody
	if err := decodeJSONWithin(w, r, &body, maxTerrainBodyBytes); err != nil {
		return response{}, err
	}
	details, err := body.details()
	if err != nil {
		return response{}, err
	}
	view, err := h.deps.Adjust.Execute(r.Context(), usecase.AdjustFieldTerrainInput{
		Actor: actor, FarmID: farmID, FieldID: fieldID, Details: details,
	})
	if err != nil {
		return response{}, err
	}
	return okResponse(fieldTerrainOf(view)), nil
}

func (h *TerrainHandler) removeFieldTerrainAdjustment(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, fieldID, err := fieldPath(r)
	if err != nil {
		return response{}, err
	}
	view, err := h.deps.RemoveAdjustment.Execute(r.Context(), usecase.RemoveFieldTerrainAdjustmentInput{Actor: actor, FarmID: farmID, FieldID: fieldID})
	if err != nil {
		return response{}, err
	}
	return okResponse(fieldTerrainOf(view)), nil
}
