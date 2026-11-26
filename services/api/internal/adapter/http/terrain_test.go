package httpadapter_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const terrainFieldID = "00000000-0000-4000-8000-0000000000c1"

// terrainStub stands for every terrain use case: it records the call and
// answers with a canned view or error.
type terrainStub struct {
	calls     int
	farmIn    usecase.GetFarmTerrainInput
	calcIn    usecase.CalculateFarmTerrainInput
	fieldIn   usecase.GetFieldTerrainInput
	adjustIn  usecase.AdjustFieldTerrainInput
	removeIn  usecase.RemoveFieldTerrainAdjustmentInput
	farmView  usecase.FarmTerrainView
	fieldView usecase.FieldTerrainView
	err       error
}

type (
	stubGetFarm   struct{ *terrainStub }
	stubCalculate struct{ *terrainStub }
	stubGetField  struct{ *terrainStub }
	stubAdjust    struct{ *terrainStub }
	stubRemove    struct{ *terrainStub }
)

func (s stubGetFarm) Execute(_ context.Context, in usecase.GetFarmTerrainInput) (usecase.FarmTerrainView, error) {
	s.calls++
	s.farmIn = in
	return s.farmView, s.err
}

func (s stubCalculate) Execute(_ context.Context, in usecase.CalculateFarmTerrainInput) (usecase.FarmTerrainView, error) {
	s.calls++
	s.calcIn = in
	return s.farmView, s.err
}

func (s stubGetField) Execute(_ context.Context, in usecase.GetFieldTerrainInput) (usecase.FieldTerrainView, error) {
	s.calls++
	s.fieldIn = in
	return s.fieldView, s.err
}

func (s stubAdjust) Execute(_ context.Context, in usecase.AdjustFieldTerrainInput) (usecase.FieldTerrainView, error) {
	s.calls++
	s.adjustIn = in
	return s.fieldView, s.err
}

func (s stubRemove) Execute(_ context.Context, in usecase.RemoveFieldTerrainAdjustmentInput) (usecase.FieldTerrainView, error) {
	s.calls++
	s.removeIn = in
	return s.fieldView, s.err
}

func newTerrainEnv(t *testing.T, stub *terrainStub) *apiEnv {
	t.Helper()
	e := newAPIEnv(t)
	logger := slog.New(slog.NewTextHandler(e.logs, nil))
	e.handler = httpadapter.NewRouter(httpadapter.RouterDeps{
		Logger:             logger,
		Liveness:           usecase.CheckLiveness{},
		Startup:            fakeProbe{report: up},
		Readiness:          fakeReadiness{report: up},
		ProbeTimeout:       time.Second,
		CORSAllowedOrigins: []string{allowedOrigin},
		Authenticator:      fakeAuthenticator{db: e.db},
		Groups: []httpadapter.RouteGroup{httpadapter.NewTerrainHandler(logger, httpadapter.TerrainDeps{
			GetFarm: stubGetFarm{stub}, Calculate: stubCalculate{stub}, GetField: stubGetField{stub},
			Adjust: stubAdjust{stub}, RemoveAdjustment: stubRemove{stub},
		})},
	})
	return e
}

func ptr[T any](value T) *T { return &value }

func readyFieldView() usecase.FieldTerrainView {
	calculatedAt := time.Date(2026, 11, 20, 15, 4, 5, 0, time.FixedZone("BRT", -3*3600))
	adjustedAt := time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC)
	p90, aspect := 14.1, domain.AspectSoutheast
	areaAbove := 12.5
	calculation := &domain.TerrainCalculation{
		Measurement: domain.TerrainMeasurement{
			SlopeMeanPct: 9.4, SlopeP90Pct: &p90, SlopeMaxPct: 21.7,
			Class: domain.SlopeUndulating, Position: domain.PositionSlope, Aspect: &aspect,
		},
		ElevationMinM: 812.5, ElevationMaxM: 843.25,
		Profile:        []domain.ProfilePoint{{DistanceM: 0, ElevationM: 843.25}, {DistanceM: 120, ElevationM: 812.5}},
		ElevationModel: domain.ElevationModel{Source: "Copernicus DEM GLO-30", ResolutionM: 30, Version: "GLO-30 COG 2022-05"},
		CalculatedAt:   calculatedAt,
	}
	adjustment := domain.TerrainAdjustment{
		Origin: domain.OriginFieldAssessment, SlopeMeanPct: 11, SlopeMaxPct: 25, Class: domain.SlopeUndulating,
		Position: domain.PositionHilltop, Justification: "Visita de campo", AdjustedBy: domain.UserID(adminAID), AdjustedAt: adjustedAt,
	}
	manual := adjustment.Measurement()
	source := domain.SourceManual
	return usecase.FieldTerrainView{
		FieldID: domain.FieldID(terrainFieldID), FarmID: domain.FarmID(farmA1),
		Status: domain.TerrainReady, Source: &source, Effective: &manual, Calculated: calculation,
		ClassAreas: []domain.SlopeClassArea{
			{Class: domain.SlopeFlat, AreaHa: 1, SharePct: 10},
			{Class: domain.SlopeGentlyUndulating, AreaHa: 2, SharePct: 20},
			{Class: domain.SlopeUndulating, AreaHa: 6, SharePct: 60},
			{Class: domain.SlopeStronglyUndulating, AreaHa: 1, SharePct: 10},
			{Class: domain.SlopeMountainous, AreaHa: 0, SharePct: 0},
		},
		Adjustment:         &usecase.TerrainAdjustmentView{Adjustment: adjustment, AdjustedByName: "Ana Admin"},
		AdjustmentOutdated: true,
		MethodFits: []domain.MethodFit{
			{Method: domain.MethodKnapsack, LimitSource: domain.LimitSourceReference, Status: domain.FitCompatible},
			{Method: domain.MethodTractorBoom, SlopeLimitPct: ptr(20.0), LimitSource: domain.LimitSourceReference, Status: domain.FitRestricted, AreaAboveLimitPct: &areaAbove},
		},
	}
}

const readyFieldJSON = `{
  "field_id": "00000000-0000-4000-8000-0000000000c1",
  "farm_id": "00000000-0000-4000-8000-0000000000f1",
  "status": "ready",
  "failure_reason": null,
  "source": "manual",
  "effective": {"slope_mean_pct": 11, "slope_p90_pct": null, "slope_max_pct": 25, "slope_class": "undulating", "position": "hilltop", "aspect": null},
  "calculated": {
    "measurement": {"slope_mean_pct": 9.4, "slope_p90_pct": 14.1, "slope_max_pct": 21.7, "slope_class": "undulating", "position": "slope", "aspect": "southeast"},
    "elevation_min_m": 812.5,
    "elevation_max_m": 843.25,
    "class_areas": [
      {"slope_class": "flat", "area_ha": 1, "share_pct": 10},
      {"slope_class": "gently_undulating", "area_ha": 2, "share_pct": 20},
      {"slope_class": "undulating", "area_ha": 6, "share_pct": 60},
      {"slope_class": "strongly_undulating", "area_ha": 1, "share_pct": 10},
      {"slope_class": "mountainous", "area_ha": 0, "share_pct": 0}
    ],
    "profile": [{"distance_m": 0, "elevation_m": 843.25}, {"distance_m": 120, "elevation_m": 812.5}],
    "elevation_model": {"source": "Copernicus DEM GLO-30", "resolution_m": 30, "version": "GLO-30 COG 2022-05"},
    "calculated_at": "2026-11-20T18:04:05Z"
  },
  "adjustment": {
    "origin": "field_assessment",
    "measurement": {"slope_mean_pct": 11, "slope_p90_pct": null, "slope_max_pct": 25, "slope_class": "undulating", "position": "hilltop", "aspect": null},
    "justification": "Visita de campo",
    "adjusted_by": {"id": "00000000-0000-4000-8000-000000000002", "name": "Ana Admin"},
    "adjusted_at": "2026-11-21T09:30:00Z"
  },
  "adjustment_outdated": true,
  "method_fits": [
    {"method": "knapsack", "slope_limit_pct": null, "limit_source": "reference", "status": "compatible", "area_above_limit_pct": null},
    {"method": "tractor_boom", "slope_limit_pct": 20, "limit_source": "reference", "status": "restricted", "area_above_limit_pct": 12.5}
  ]
}`

const notCalculatedFieldJSON = `{
  "field_id": "00000000-0000-4000-8000-0000000000c1",
  "farm_id": "00000000-0000-4000-8000-0000000000f1",
  "status": "not_calculated",
  "failure_reason": null,
  "source": null,
  "effective": null,
  "calculated": null,
  "adjustment": null,
  "adjustment_outdated": false,
  "method_fits": []
}`

func notCalculatedFieldView() usecase.FieldTerrainView {
	return usecase.FieldTerrainView{
		FieldID: domain.FieldID(terrainFieldID), FarmID: domain.FarmID(farmA1),
		Status: domain.TerrainNotCalculated, MethodFits: []domain.MethodFit{},
	}
}

func sameJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("response is not JSON: %v\n%s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("bad expectation: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("body differs\n got: %s\nwant: %s", got, want)
	}
}

func farmTerrainView() usecase.FarmTerrainView {
	calculatedAt := time.Date(2026, 11, 20, 15, 4, 5, 0, time.UTC)
	p90, aspect := 14.1, domain.AspectNorth
	measurement := domain.TerrainMeasurement{
		SlopeMeanPct: 9.4, SlopeP90Pct: &p90, SlopeMaxPct: 21.7, Class: domain.SlopeUndulating, Position: domain.PositionSlope, Aspect: &aspect,
	}
	source, failure := domain.SourceCalculated, "bloco de elevação indisponível"
	return usecase.FarmTerrainView{
		FarmID: domain.FarmID(farmA1),
		Summary: domain.FarmTerrainSummary{
			FieldCount: 2, CalculatedFieldCount: 1, SlopeMeanPct: ptr(9.4), RestrictedFieldCount: 1,
			ElevationMinM: ptr(812.5), ElevationMaxM: ptr(843.25), CalculatedAt: &calculatedAt,
		},
		Progress: &usecase.TerrainProgress{Total: 3, Done: 1},
		Fields: []usecase.FieldTerrainRowView{
			{
				FieldID: domain.FieldID(terrainFieldID), FieldName: "Talhão 1", Block: ptr("A"), AreaHa: 10, Status: domain.TerrainReady,
				Source: &source, Effective: &measurement, ElevationMinM: ptr(812.5), ElevationMaxM: ptr(843.25), HasRestrictedMethod: true,
			},
			{FieldID: "00000000-0000-4000-8000-0000000000c2", FieldName: "Talhão 2", AreaHa: 4.5, Status: domain.TerrainFailed, FailureReason: &failure},
		},
	}
}

const farmTerrainJSON = `{
  "farm_id": "00000000-0000-4000-8000-0000000000f1",
  "summary": {
    "field_count": 2, "calculated_field_count": 1, "slope_mean_pct": 9.4, "restricted_field_count": 1,
    "elevation_min_m": 812.5, "elevation_max_m": 843.25, "calculated_at": "2026-11-20T15:04:05Z"
  },
  "progress": {"total": 3, "done": 1},
  "items": [
    {
      "field_id": "00000000-0000-4000-8000-0000000000c1", "field_name": "Talhão 1", "block": "A", "area_ha": 10,
      "status": "ready", "failure_reason": null, "source": "calculated",
      "measurement": {"slope_mean_pct": 9.4, "slope_p90_pct": 14.1, "slope_max_pct": 21.7, "slope_class": "undulating", "position": "slope", "aspect": "north"},
      "elevation_min_m": 812.5, "elevation_max_m": 843.25, "has_restricted_method": true
    },
    {
      "field_id": "00000000-0000-4000-8000-0000000000c2", "field_name": "Talhão 2", "block": null, "area_ha": 4.5,
      "status": "failed", "failure_reason": "bloco de elevação indisponível", "source": null, "measurement": null,
      "elevation_min_m": null, "elevation_max_m": null, "has_restricted_method": false
    }
  ]
}`

func TestFarmTerrainEndpoint(t *testing.T) {
	stub := &terrainStub{farmView: farmTerrainView()}
	e := newTerrainEnv(t, stub)

	got := e.call(operatorAID, http.MethodGet, "/farms/"+farmA1+"/terrain", nil)
	if got.status != http.StatusOK {
		t.Fatalf("status %d: %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, farmTerrainJSON)
	if got.header.Get("Cache-Control") != "no-store" || !strings.HasPrefix(got.header.Get("Content-Type"), "application/json") {
		t.Errorf("headers: %v", got.header)
	}

	t.Run("no calculation in progress and nothing calculated", func(t *testing.T) {
		stub.farmView = usecase.FarmTerrainView{FarmID: domain.FarmID(farmA1)}
		body := e.call(adminAID, http.MethodGet, "/farms/"+farmA1+"/terrain", nil).object(t)
		summary, _ := body["summary"].(map[string]any)
		items, isList := body["items"].([]any)
		if body["progress"] != nil || summary["calculated_at"] != nil || summary["slope_mean_pct"] != nil || !isList || len(items) != 0 {
			t.Errorf("empty farm: %v", body)
		}
	})
	t.Run("not found", func(t *testing.T) {
		stub.err = usecase.ErrNotFound
		defer func() { stub.err = nil }()
		if got := e.call(adminAID, http.MethodGet, "/farms/"+farmA1+"/terrain", nil); got.status != http.StatusNotFound {
			t.Errorf("status %d", got.status)
		}
		if got := e.call(adminAID, http.MethodGet, "/farms/not-a-uuid/terrain", nil); got.status != http.StatusNotFound {
			t.Errorf("malformed id: %d", got.status)
		}
	})
	t.Run("internal error is not leaked", func(t *testing.T) {
		stub.err = context.DeadlineExceeded
		defer func() { stub.err = nil }()
		got := e.call(adminAID, http.MethodGet, "/farms/"+farmA1+"/terrain", nil)
		if got.status != http.StatusInternalServerError || got.errorCode(t) != "internal_error" {
			t.Errorf("status %d: %s", got.status, got.raw)
		}
	})
}

func TestCalculateFarmTerrainEndpoint(t *testing.T) {
	stub := &terrainStub{farmView: farmTerrainView()}
	e := newTerrainEnv(t, stub)
	path := "/farms/" + farmA1 + "/terrain/calculations"

	tests := []struct {
		name      string
		body      any
		wantScope usecase.TerrainRunScope
	}{
		{"empty body means missing", nil, ""},
		{"blank body means missing", "  \n", ""},
		{"empty object means missing", `{}`, ""},
		{"null scope means missing", `{"scope": null}`, ""},
		{"scope missing", `{"scope": "missing"}`, usecase.TerrainScopeMissing},
		{"scope all", `{"scope": "all"}`, usecase.TerrainScopeAll},
		{"unknown value goes to the use case", `{"scope": "everything"}`, "everything"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub.calcIn = usecase.CalculateFarmTerrainInput{}
			got := e.call(adminAID, http.MethodPost, path, tt.body)
			if got.status != http.StatusAccepted {
				t.Fatalf("status %d: %s", got.status, got.raw)
			}
			sameJSON(t, got.raw, farmTerrainJSON)
			if stub.calcIn.Scope != tt.wantScope || stub.calcIn.FarmID != domain.FarmID(farmA1) || string(stub.calcIn.Actor.UserID) != adminAID {
				t.Errorf("use case input: %+v", stub.calcIn)
			}
			if got.header.Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control: %q", got.header.Get("Cache-Control"))
			}
		})
	}

	t.Run("invalid scope becomes 422 on scope", func(t *testing.T) {
		stub.err = domain.Invalid("scope", "escopo deve ser missing ou all")
		defer func() { stub.err = nil }()
		got := e.call(adminAID, http.MethodPost, path, `{"scope": "everything"}`)
		if got.status != http.StatusUnprocessableEntity || !reflect.DeepEqual(got.errorFields(t), []string{"scope"}) {
			t.Errorf("status %d: %s", got.status, got.raw)
		}
	})
	t.Run("malformed bodies", func(t *testing.T) {
		for name, body := range map[string]string{
			"invalid json":  `{"scope": `,
			"unknown field": `{"scope": "all", "force": true}`,
			"wrong type":    `{"scope": 3}`,
			"two objects":   `{} {}`,
		} {
			got := e.call(adminAID, http.MethodPost, path, body)
			if got.status != http.StatusUnprocessableEntity || got.errorCode(t) != "validation_failed" {
				t.Errorf("%s: status %d: %s", name, got.status, got.raw)
			}
		}
	})
	t.Run("forbidden without registry.write", func(t *testing.T) {
		stub.err = usecase.ErrForbidden
		defer func() { stub.err = nil }()
		if got := e.call(operatorAID, http.MethodPost, path, nil); got.status != http.StatusForbidden || got.errorCode(t) != "forbidden" {
			t.Errorf("status %d: %s", got.status, got.raw)
		}
	})
	t.Run("not found", func(t *testing.T) {
		stub.err = usecase.ErrNotFound
		defer func() { stub.err = nil }()
		if got := e.call(adminAID, http.MethodPost, path, nil); got.status != http.StatusNotFound {
			t.Errorf("status %d", got.status)
		}
	})
}

func TestFieldTerrainEndpoint(t *testing.T) {
	stub := &terrainStub{fieldView: readyFieldView()}
	e := newTerrainEnv(t, stub)
	path := "/farms/" + farmA1 + "/fields/" + terrainFieldID + "/terrain"

	got := e.call(operatorAID, http.MethodGet, path, nil)
	if got.status != http.StatusOK {
		t.Fatalf("status %d: %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, readyFieldJSON)
	if got.header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control: %q", got.header.Get("Cache-Control"))
	}
	if stub.fieldIn.FarmID != domain.FarmID(farmA1) || stub.fieldIn.FieldID != domain.FieldID(terrainFieldID) || string(stub.fieldIn.Actor.UserID) != operatorAID {
		t.Errorf("use case input: %+v", stub.fieldIn)
	}

	t.Run("not calculated", func(t *testing.T) {
		stub.fieldView = notCalculatedFieldView()
		sameJSON(t, e.call(adminAID, http.MethodGet, path, nil).raw, notCalculatedFieldJSON)
	})
	t.Run("nil method fits and class areas are still arrays", func(t *testing.T) {
		view := readyFieldView()
		view.MethodFits, view.ClassAreas = nil, nil
		stub.fieldView = view
		body := e.call(adminAID, http.MethodGet, path, nil).object(t)
		calculated, _ := body["calculated"].(map[string]any)
		if _, ok := body["method_fits"].([]any); !ok {
			t.Errorf("method_fits: %v", body["method_fits"])
		}
		if _, ok := calculated["class_areas"].([]any); !ok {
			t.Errorf("class_areas: %v", calculated["class_areas"])
		}
	})
	t.Run("failed keeps the reason", func(t *testing.T) {
		reason := "bloco de elevação indisponível"
		stub.fieldView = usecase.FieldTerrainView{FieldID: domain.FieldID(terrainFieldID), FarmID: domain.FarmID(farmA1), Status: domain.TerrainFailed, FailureReason: &reason}
		body := e.call(adminAID, http.MethodGet, path, nil).object(t)
		if body["status"] != "failed" || body["failure_reason"] != reason {
			t.Errorf("failed body: %v", body)
		}
	})
	t.Run("not found", func(t *testing.T) {
		stub.err = usecase.ErrNotFound
		defer func() { stub.err = nil }()
		if got := e.call(adminAID, http.MethodGet, path, nil); got.status != http.StatusNotFound {
			t.Errorf("status %d", got.status)
		}
		if got := e.call(adminAID, http.MethodGet, "/farms/"+farmA1+"/fields/xyz/terrain", nil); got.status != http.StatusNotFound {
			t.Errorf("malformed field id: %d", got.status)
		}
	})
}

func TestAdjustFieldTerrainEndpoint(t *testing.T) {
	stub := &terrainStub{fieldView: readyFieldView()}
	e := newTerrainEnv(t, stub)
	path := "/farms/" + farmA1 + "/fields/" + terrainFieldID + "/terrain/adjustment"
	valid := map[string]any{
		"origin": "field_assessment", "slope_mean_pct": 11, "slope_max_pct": 25, "position": "hilltop", "justification": "Visita de campo",
	}

	got := e.call(adminAID, http.MethodPut, path, valid)
	if got.status != http.StatusOK {
		t.Fatalf("status %d: %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, readyFieldJSON)
	want := domain.TerrainAdjustmentDetails{
		Origin: domain.OriginFieldAssessment, SlopeMeanPct: 11, SlopeMaxPct: 25, Position: domain.PositionHilltop, Justification: "Visita de campo",
	}
	if stub.adjustIn.Details != want || stub.adjustIn.FieldID != domain.FieldID(terrainFieldID) || stub.adjustIn.FarmID != domain.FarmID(farmA1) {
		t.Errorf("use case input: %+v", stub.adjustIn)
	}

	t.Run("zero is a value, absence is not", func(t *testing.T) {
		zero := map[string]any{"origin": "field_assessment", "slope_mean_pct": 0, "slope_max_pct": 0, "position": "lowland", "justification": "plano"}
		if got := e.call(adminAID, http.MethodPut, path, zero); got.status != http.StatusOK {
			t.Fatalf("zero values: %d %s", got.status, got.raw)
		}
		missing := map[string]any{"origin": "field_assessment", "position": "lowland", "justification": "plano"}
		got := e.call(adminAID, http.MethodPut, path, missing)
		fields := got.errorFields(t)
		if got.status != http.StatusUnprocessableEntity || !reflect.DeepEqual(fields, []string{"slope_mean_pct", "slope_max_pct"}) {
			t.Errorf("missing numbers: %d %s", got.status, got.raw)
		}
	})
	t.Run("domain validation lists every field", func(t *testing.T) {
		stub.err = &domain.ValidationError{Fields: []domain.FieldError{
			{Field: "origin", Message: "x"}, {Field: "slope_mean_pct", Message: "x"}, {Field: "slope_max_pct", Message: "x"},
			{Field: "position", Message: "x"}, {Field: "justification", Message: "x"},
		}}
		defer func() { stub.err = nil }()
		got := e.call(adminAID, http.MethodPut, path, valid)
		want := []string{"origin", "slope_mean_pct", "slope_max_pct", "position", "justification"}
		if got.status != http.StatusUnprocessableEntity || !reflect.DeepEqual(got.errorFields(t), want) {
			t.Errorf("status %d: %s", got.status, got.raw)
		}
	})
	t.Run("malformed bodies", func(t *testing.T) {
		for name, body := range map[string]string{
			"empty":         "",
			"invalid json":  `{"origin":`,
			"unknown field": `{"origin":"field_assessment","slope_mean_pct":1,"slope_max_pct":2,"position":"slope","justification":"x","note":"y"}`,
			"wrong type":    `{"origin":"field_assessment","slope_mean_pct":"alta","slope_max_pct":2,"position":"slope","justification":"x"}`,
		} {
			got := e.call(adminAID, http.MethodPut, path, body)
			if got.status != http.StatusUnprocessableEntity {
				t.Errorf("%s: status %d: %s", name, got.status, got.raw)
			}
		}
	})
	t.Run("forbidden without registry.write", func(t *testing.T) {
		stub.err = usecase.ErrForbidden
		defer func() { stub.err = nil }()
		if got := e.call(operatorAID, http.MethodPut, path, valid); got.status != http.StatusForbidden {
			t.Errorf("status %d", got.status)
		}
	})
	t.Run("not found", func(t *testing.T) {
		stub.err = usecase.ErrNotFound
		defer func() { stub.err = nil }()
		if got := e.call(adminAID, http.MethodPut, path, valid); got.status != http.StatusNotFound {
			t.Errorf("status %d", got.status)
		}
	})
}

func TestRemoveFieldTerrainAdjustmentEndpoint(t *testing.T) {
	stub := &terrainStub{fieldView: notCalculatedFieldView()}
	e := newTerrainEnv(t, stub)
	path := "/farms/" + farmA1 + "/fields/" + terrainFieldID + "/terrain/adjustment"

	got := e.call(adminAID, http.MethodDelete, path, nil)
	if got.status != http.StatusOK {
		t.Fatalf("status %d: %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, notCalculatedFieldJSON)
	if stub.removeIn.FieldID != domain.FieldID(terrainFieldID) || stub.removeIn.FarmID != domain.FarmID(farmA1) {
		t.Errorf("use case input: %+v", stub.removeIn)
	}
	if got.header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control: %q", got.header.Get("Cache-Control"))
	}
	for name, tt := range map[string]struct {
		err    error
		status int
	}{
		"no adjustment": {usecase.ErrNotFound, http.StatusNotFound},
		"forbidden":     {usecase.ErrForbidden, http.StatusForbidden},
		"internal":      {context.Canceled, http.StatusInternalServerError},
	} {
		stub.err = tt.err
		if got := e.call(adminAID, http.MethodDelete, path, nil); got.status != tt.status {
			t.Errorf("%s: status %d", name, got.status)
		}
	}
}

// Nothing runs without a session: the use cases are never reached.
func TestTerrainRoutesRequireAuthentication(t *testing.T) {
	stub := &terrainStub{fieldView: readyFieldView(), farmView: farmTerrainView()}
	e := newTerrainEnv(t, stub)
	fieldPath := "/farms/" + farmA1 + "/fields/" + terrainFieldID + "/terrain"
	routes := []struct{ method, path string }{
		{http.MethodGet, "/farms/" + farmA1 + "/terrain"},
		{http.MethodPost, "/farms/" + farmA1 + "/terrain/calculations"},
		{http.MethodGet, fieldPath},
		{http.MethodPut, fieldPath + "/adjustment"},
		{http.MethodDelete, fieldPath + "/adjustment"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			got := e.call("", route.method, route.path, nil)
			if got.status != http.StatusUnauthorized || got.errorCode(t) != "unauthorized" {
				t.Fatalf("status %d: %s", got.status, got.raw)
			}
			if got.header.Get("WWW-Authenticate") == "" {
				t.Error("401 must carry WWW-Authenticate")
			}
		})
	}
	if stub.calls != 0 {
		t.Error("a use case ran without authentication")
	}
	if got := e.call(adminAID, http.MethodPatch, fieldPath, nil); got.status != http.StatusMethodNotAllowed {
		t.Errorf("PATCH on terrain: %d", got.status)
	}
}
