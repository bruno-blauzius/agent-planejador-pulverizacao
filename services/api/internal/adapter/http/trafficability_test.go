package httpadapter_test

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	apispec "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/api"
	httpadapter "github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/http"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	gaugeID      = "00000000-0000-4000-8000-0000000000d1"
	conditionsAt = "2026-11-26T07:00:00-03:00"
)

// conditionsStub stands for every use case of the Trafficability tag: it
// records the last input and answers with canned values or err.
type conditionsStub struct {
	listGaugesIn    usecase.ListRainGaugesInput
	createGaugeIn   usecase.CreateRainGaugeInput
	updateGaugeIn   usecase.UpdateRainGaugeInput
	listReadingsIn  usecase.ListRainReadingsInput
	recordReadingIn usecase.RecordRainReadingInput
	inspectionIn    usecase.RecordSoilInspectionInput
	getProfileIn    usecase.GetSoilProfileInput
	updateProfileIn usecase.UpdateSoilProfileInput
	trafficIn       usecase.GetFieldTrafficabilityInput
	calls           int

	gauge      domain.RainGauge
	reading    usecase.StoredRainReading
	inspection usecase.SoilInspectionView
	profile    usecase.SoilProfileView
	traffic    usecase.FieldTrafficabilityView
	err        error
}

type (
	stubListGauges    struct{ *conditionsStub }
	stubCreateGauge   struct{ *conditionsStub }
	stubUpdateGauge   struct{ *conditionsStub }
	stubListReadings  struct{ *conditionsStub }
	stubRecordReading struct{ *conditionsStub }
	stubInspection    struct{ *conditionsStub }
	stubGetProfile    struct{ *conditionsStub }
	stubUpdateProfile struct{ *conditionsStub }
	stubTraffic       struct{ *conditionsStub }
)

func (s stubListGauges) Execute(_ context.Context, in usecase.ListRainGaugesInput) ([]domain.RainGauge, error) {
	s.calls++
	s.listGaugesIn = in
	return []domain.RainGauge{s.gauge}, s.err
}

func (s stubCreateGauge) Execute(_ context.Context, in usecase.CreateRainGaugeInput) (domain.RainGauge, error) {
	s.calls++
	s.createGaugeIn = in
	return s.gauge, s.err
}

func (s stubUpdateGauge) Execute(_ context.Context, in usecase.UpdateRainGaugeInput) (domain.RainGauge, error) {
	s.calls++
	s.updateGaugeIn = in
	return s.gauge, s.err
}

func (s stubListReadings) Execute(_ context.Context, in usecase.ListRainReadingsInput) ([]usecase.StoredRainReading, error) {
	s.calls++
	s.listReadingsIn = in
	return []usecase.StoredRainReading{s.reading}, s.err
}

func (s stubRecordReading) Execute(_ context.Context, in usecase.RecordRainReadingInput) (usecase.StoredRainReading, error) {
	s.calls++
	s.recordReadingIn = in
	return s.reading, s.err
}

func (s stubInspection) Execute(_ context.Context, in usecase.RecordSoilInspectionInput) (usecase.SoilInspectionView, error) {
	s.calls++
	s.inspectionIn = in
	return s.inspection, s.err
}

func (s stubGetProfile) Execute(_ context.Context, in usecase.GetSoilProfileInput) (usecase.SoilProfileView, error) {
	s.calls++
	s.getProfileIn = in
	return s.profile, s.err
}

func (s stubUpdateProfile) Execute(_ context.Context, in usecase.UpdateSoilProfileInput) (usecase.SoilProfileView, error) {
	s.calls++
	s.updateProfileIn = in
	return s.profile, s.err
}

func (s stubTraffic) Execute(_ context.Context, in usecase.GetFieldTrafficabilityInput) (usecase.FieldTrafficabilityView, error) {
	s.calls++
	s.trafficIn = in
	return s.traffic, s.err
}

func newConditionsEnv(t *testing.T, stub *conditionsStub) *apiEnv {
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
		Groups: []httpadapter.RouteGroup{httpadapter.NewTrafficabilityHandler(logger, httpadapter.TrafficabilityDeps{
			ListGauges: stubListGauges{stub}, CreateGauge: stubCreateGauge{stub}, UpdateGauge: stubUpdateGauge{stub},
			ListReadings: stubListReadings{stub}, RecordReading: stubRecordReading{stub}, RecordInspection: stubInspection{stub},
			GetSoilProfile: stubGetProfile{stub}, UpdateSoilProfile: stubUpdateProfile{stub}, GetTrafficability: stubTraffic{stub},
		})},
	})
	return e
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func sampleStub(t *testing.T) *conditionsStub {
	at := mustTime(t, conditionsAt)
	note := "barro"
	return &conditionsStub{
		gauge: domain.RainGauge{
			ID: gaugeID, CompanyID: companyAID, FarmID: farmA1, Name: "Sede",
			Location: &domain.GeoPoint{Lat: -21.1, Lng: -47.2}, CreatedAt: at, UpdatedAt: at, Version: 2,
		},
		reading: usecase.StoredRainReading{
			Reading:        domain.RainReading{ID: "r1", GaugeID: gaugeID, RainMm: 12.5, ObservedAt: at, RecordedBy: operatorAID, RecordedAt: at},
			RecordedByName: "Olga Operadora",
		},
		inspection: usecase.SoilInspectionView{
			Inspection: domain.FieldSoilInspection{
				ID: "i1", FarmID: farmA1, FieldID: terrainFieldID, State: domain.SoilMoist, InspectedAt: at, InspectedBy: operatorAID, Note: &note,
			},
			InspectedByName: "Olga Operadora", ValidUntil: at.Add(12 * time.Hour),
		},
	}
}

// The Trafficability operations of the contract and the routes of the
// handler are the same set.
func TestTrafficabilityRoutesMatchTheContract(t *testing.T) {
	pathLine := regexp.MustCompile(`^  (/\S+):$`)
	methodLine := regexp.MustCompile(`^    (get|put|post|delete|patch):$`)
	var path, method string
	var operations []string
	for _, line := range strings.Split(string(apispec.Spec), "\n") {
		line = strings.TrimRight(line, "\r")
		if m := pathLine.FindStringSubmatch(line); m != nil {
			path, method = m[1], ""
		}
		if m := methodLine.FindStringSubmatch(line); m != nil {
			method = strings.ToUpper(m[1])
		}
		if strings.TrimSpace(line) == "tags: [Trafficability]" && path != "" && method != "" {
			operations = append(operations, method+" "+path)
		}
	}
	want := []string{
		"GET /farms/{farmId}/rain-gauges",
		"POST /farms/{farmId}/rain-gauges",
		"PATCH /farms/{farmId}/rain-gauges/{gaugeId}",
		"GET /farms/{farmId}/rain-gauges/{gaugeId}/readings",
		"POST /farms/{farmId}/rain-gauges/{gaugeId}/readings",
		"POST /farms/{farmId}/fields/{fieldId}/soil-inspections",
		"GET /farms/{farmId}/fields/{fieldId}/soil-profile",
		"PUT /farms/{farmId}/fields/{fieldId}/soil-profile",
		"GET /farms/{farmId}/fields/{fieldId}/trafficability",
	}
	slices.Sort(operations)
	slices.Sort(want)
	if !slices.Equal(operations, want) {
		t.Fatalf("Trafficability operations in the contract:\n got %v\nwant %v", operations, want)
	}
	e := newConditionsEnv(t, &conditionsStub{})
	for _, operation := range operations {
		verb, template, _ := strings.Cut(operation, " ")
		concrete := strings.NewReplacer("{farmId}", farmA1, "{fieldId}", terrainFieldID, "{gaugeId}", gaugeID).Replace(template)
		if got := e.call("", verb, concrete, nil); got.status != http.StatusUnauthorized {
			t.Errorf("%s is not a registered protected route: status %d", operation, got.status)
		}
	}
}

func TestRainGaugeEndpoints(t *testing.T) {
	stub := sampleStub(t)
	e := newConditionsEnv(t, stub)
	base := "/farms/" + farmA1 + "/rain-gauges"
	gaugeJSON := `{"id": "` + gaugeID + `", "farm_id": "` + farmA1 + `", "name": "Sede", "location": {"lat": -21.1, "lng": -47.2},
	  "created_at": "2026-11-26T07:00:00-03:00", "updated_at": "2026-11-26T07:00:00-03:00", "version": 2}`

	got := e.call(operatorAID, http.MethodGet, base, nil)
	if got.status != http.StatusOK {
		t.Fatalf("list: %d %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, `{"items": [`+gaugeJSON+`]}`)

	got = e.call(adminAID, http.MethodPost, base, `{"name": "Sede", "location": {"lat": -21.1, "lng": -47.2}}`)
	if got.status != http.StatusCreated {
		t.Fatalf("create: %d %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, gaugeJSON)
	if in := stub.createGaugeIn; in.Details.Name != "Sede" || in.Details.Location == nil || in.Details.Location.Lat != -21.1 || in.FarmID != farmA1 {
		t.Errorf("create input = %+v", in)
	}
	if got := e.call(adminAID, http.MethodPost, base, `{"name": "Sede", "altitude": 800}`); got.status != http.StatusUnprocessableEntity {
		t.Errorf("unknown field: %d", got.status)
	}

	patches := []struct {
		name         string
		body         string
		wantName     *string
		wantSet      bool
		wantLocation bool
	}{
		{"rename only", `{"name": "Baixada"}`, ptr("Baixada"), false, false},
		{"remove the location", `{"location": null}`, nil, true, false},
		{"move the gauge", `{"location": {"lat": -21, "lng": -47}}`, nil, true, true},
	}
	for _, tt := range patches {
		t.Run("patch: "+tt.name, func(t *testing.T) {
			got := e.call(adminAID, http.MethodPatch, base+"/"+gaugeID, tt.body)
			if got.status != http.StatusOK {
				t.Fatalf("status %d: %s", got.status, got.raw)
			}
			patch := stub.updateGaugeIn.Patch
			if (patch.Name == nil) != (tt.wantName == nil) || (tt.wantName != nil && *patch.Name != *tt.wantName) ||
				patch.LocationSet != tt.wantSet || (patch.Location != nil) != tt.wantLocation || stub.updateGaugeIn.GaugeID != gaugeID {
				t.Errorf("patch = %+v", stub.updateGaugeIn)
			}
		})
	}
	for _, body := range []string{`{"name": null}`, `{"name": 3}`, `{"location": "here"}`} {
		if got := e.call(adminAID, http.MethodPatch, base+"/"+gaugeID, body); got.status != http.StatusUnprocessableEntity {
			t.Errorf("patch %s: status %d", body, got.status)
		}
	}

	t.Run("forbidden and not found come from the use case", func(t *testing.T) {
		defer func() { stub.err = nil }()
		stub.err = usecase.ErrForbidden
		if got := e.call(operatorAID, http.MethodPost, base, `{"name": "X"}`); got.status != http.StatusForbidden {
			t.Errorf("forbidden: %d", got.status)
		}
		stub.err = usecase.ErrNotFound
		if got := e.call(adminAID, http.MethodGet, base, nil); got.status != http.StatusNotFound {
			t.Errorf("not found: %d", got.status)
		}
		if got := e.call(adminAID, http.MethodPatch, base+"/not-a-uuid", `{}`); got.status != http.StatusNotFound {
			t.Errorf("malformed gauge id: %d", got.status)
		}
	})
}

func TestRainReadingEndpoints(t *testing.T) {
	stub := sampleStub(t)
	e := newConditionsEnv(t, stub)
	path := "/farms/" + farmA1 + "/rain-gauges/" + gaugeID + "/readings"
	readingJSON := `{"id": "r1", "rain_gauge_id": "` + gaugeID + `", "rain_mm": 12.5, "observed_at": "2026-11-26T07:00:00-03:00",
	  "recorded_by": {"id": "` + operatorAID + `", "name": "Olga Operadora"}, "recorded_at": "2026-11-26T07:00:00-03:00"}`

	got := e.call(operatorAID, http.MethodPost, path, `{"rain_mm": 0, "observed_at": "2026-11-26T07:00:00-03:00"}`)
	if got.status != http.StatusCreated {
		t.Fatalf("record: %d %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, readingJSON)
	if in := stub.recordReadingIn; in.RainMm != 0 || !in.ObservedAt.Equal(mustTime(t, conditionsAt)) || in.GaugeID != gaugeID {
		t.Errorf("record input = %+v", in)
	}
	got = e.call(operatorAID, http.MethodPost, path, `{}`)
	if got.status != http.StatusUnprocessableEntity || !slices.Equal(got.errorFields(t), []string{"rain_mm", "observed_at"}) {
		t.Errorf("missing values: %d %v", got.status, got.errorFields(t))
	}

	got = e.call(operatorAID, http.MethodGet, path+"?from=2026-11-20T00:00:00Z&to=2026-11-26T00:00:00-03:00", nil)
	if got.status != http.StatusOK {
		t.Fatalf("list: %d %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, `{"items": [`+readingJSON+`]}`)
	if in := stub.listReadingsIn; !in.After.Equal(mustTime(t, "2026-11-20T00:00:00Z")) || !in.Until.Equal(mustTime(t, "2026-11-26T03:00:00Z")) {
		t.Errorf("list input = %+v", in)
	}
	e.call(operatorAID, http.MethodGet, path, nil)
	if in := stub.listReadingsIn; !in.After.IsZero() || !in.Until.IsZero() {
		t.Errorf("without parameters the use case applies the defaults: %+v", in)
	}
	got = e.call(operatorAID, http.MethodGet, path+"?from=yesterday", nil)
	if got.status != http.StatusUnprocessableEntity || !slices.Equal(got.errorFields(t), []string{"from"}) {
		t.Errorf("bad from: %d %s", got.status, got.raw)
	}
}

func TestSoilInspectionAndProfileEndpoints(t *testing.T) {
	stub := sampleStub(t)
	clay := domain.TextureClay
	stub.profile = usecase.SoilProfileView{
		FieldID: terrainFieldID, FarmID: farmA1, Texture: &clay, Gauge: &domain.RainGauge{ID: gaugeID, Name: "Sede"},
	}
	e := newConditionsEnv(t, stub)
	fieldBase := "/farms/" + farmA1 + "/fields/" + terrainFieldID

	got := e.call(operatorAID, http.MethodPost, fieldBase+"/soil-inspections", `{"state": "moist", "note": "barro"}`)
	if got.status != http.StatusCreated {
		t.Fatalf("inspection: %d %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, `{"id": "i1", "field_id": "`+terrainFieldID+`", "farm_id": "`+farmA1+`", "state": "moist",
	  "inspected_at": "2026-11-26T07:00:00-03:00", "valid_until": "2026-11-26T19:00:00-03:00",
	  "inspected_by": {"id": "`+operatorAID+`", "name": "Olga Operadora"}, "note": "barro"}`)
	if in := stub.inspectionIn; in.Details.State != domain.SoilMoist || !in.Details.InspectedAt.IsZero() || in.Details.Note == nil {
		t.Errorf("inspection input = %+v", in)
	}

	profileJSON := `{"field_id": "` + terrainFieldID + `", "farm_id": "` + farmA1 + `", "soil_texture": "clay", "rain_gauge": {"id": "` + gaugeID + `", "name": "Sede"}}`
	got = e.call(operatorAID, http.MethodGet, fieldBase+"/soil-profile", nil)
	if got.status != http.StatusOK {
		t.Fatalf("get profile: %d", got.status)
	}
	sameJSON(t, got.raw, profileJSON)

	got = e.call(adminAID, http.MethodPut, fieldBase+"/soil-profile", `{"soil_texture": "clay", "rain_gauge_id": "`+strings.ToUpper(gaugeID)+`"}`)
	if got.status != http.StatusOK {
		t.Fatalf("put profile: %d %s", got.status, got.raw)
	}
	if in := stub.updateProfileIn; in.Texture == nil || *in.Texture != clay || in.GaugeID == nil || *in.GaugeID != gaugeID {
		t.Errorf("put input = %+v", in)
	}
	got = e.call(adminAID, http.MethodPut, fieldBase+"/soil-profile", `{"soil_texture": null, "rain_gauge_id": null}`)
	if got.status != http.StatusOK || stub.updateProfileIn.Texture != nil || stub.updateProfileIn.GaugeID != nil {
		t.Errorf("clearing: %d %+v", got.status, stub.updateProfileIn)
	}
	refused := []struct {
		body   string
		fields []string
	}{
		{`{}`, []string{"soil_texture", "rain_gauge_id"}},
		{`{"soil_texture": "clay"}`, []string{"rain_gauge_id"}},
		{`{"soil_texture": null, "rain_gauge_id": "x"}`, []string{"rain_gauge_id"}},
		{`{"soil_texture": 1, "rain_gauge_id": null}`, []string{"soil_texture"}},
	}
	for _, tt := range refused {
		got := e.call(adminAID, http.MethodPut, fieldBase+"/soil-profile", tt.body)
		if got.status != http.StatusUnprocessableEntity || !slices.Equal(got.errorFields(t), tt.fields) {
			t.Errorf("%s: %d %v", tt.body, got.status, got.errorFields(t))
		}
	}
}

func trafficabilityView(t *testing.T) usecase.FieldTrafficabilityView {
	brt := time.FixedZone("BRT", -3*3600)
	asOf := mustTime(t, conditionsAt).In(brt)
	today := time.Date(2026, 11, 26, 0, 0, 0, 0, brt)
	tomorrow := today.AddDate(0, 0, 1)
	clay := domain.TextureClay
	name := "Pluviômetro da sede"
	area := 2.0
	return usecase.FieldTrafficabilityView{
		FieldID: terrainFieldID, FarmID: farmA1, AsOf: asOf,
		Soil: usecase.SoilStatusView{
			State: domain.SoilWaterlogged, Basis: domain.BasisMeasuredRain, Texture: &clay, ThresholdsSource: "heuristic",
			Rain: &domain.RainAccumulation{
				Last24hMm: 6, Last48hMm: 24, Last72hMm: 55, Last7dMm: 71, Source: domain.RainFromGauge, SourceID: gaugeID, ObservedAt: asOf,
			},
			RainSourceName: &name,
			ModelMoisture:  &domain.ModelSoilMoisture{VolumetricM3M3: 0.18, FieldCapacityM3M3: 0.36},
		},
		RainDaily: []usecase.DailyRainEntry{
			{Date: today, RainMm: 6, Kind: usecase.RainMeasured, Source: domain.RainFromGauge},
			{Date: tomorrow, RainMm: 2, Kind: usecase.RainForecast, Source: domain.RainFromModel},
		},
		Plan: domain.TrafficabilityPlan{
			Days: []domain.TrafficabilityDay{{Date: today, SoilState: domain.SoilWaterlogged, Methods: []domain.MethodEligibility{
				{Method: domain.MethodDrone, Verdict: domain.VerdictApt, Reasons: []domain.EligibilityReason{}},
				{Method: domain.MethodAirblast, Verdict: domain.VerdictUnfit, Reasons: []domain.EligibilityReason{domain.ReasonSoilWaterlogged}, AreaAboveLimitPct: &area},
			}}},
			Releases: []domain.MethodRelease{
				{Method: domain.MethodAirblast, RequiredState: domain.SoilIdeal, Date: &tomorrow},
				{Method: domain.MethodKnapsack, RequiredState: domain.SoilMoist},
			},
		},
		DataIssues: []usecase.TrafficabilityDataIssue{usecase.IssueTerrainNotCalculated},
	}
}

const trafficabilityJSON = `{
  "field_id": "00000000-0000-4000-8000-0000000000c1",
  "farm_id": "00000000-0000-4000-8000-0000000000f1",
  "as_of": "2026-11-26T07:00:00-03:00",
  "soil": {
    "state": "waterlogged", "basis": "measured_rain", "texture": "clay", "thresholds_source": "heuristic",
    "rain": {"last_24h_mm": 6, "last_48h_mm": 24, "last_72h_mm": 55, "last_7d_mm": 71, "source": "gauge",
             "source_name": "Pluviômetro da sede", "observed_at": "2026-11-26T07:00:00-03:00"},
    "model_moisture": {"volumetric_m3m3": 0.18, "field_capacity_m3m3": 0.36, "field_capacity_pct": 50},
    "inspection": null
  },
  "rain_daily": [
    {"date": "2026-11-26", "rain_mm": 6, "kind": "measured", "source": "gauge"},
    {"date": "2026-11-27", "rain_mm": 2, "kind": "forecast", "source": "model"}
  ],
  "days": [
    {"date": "2026-11-26", "soil_state": "waterlogged", "methods": [
      {"method": "drone", "verdict": "apt", "reasons": [], "area_above_limit_pct": null},
      {"method": "airblast", "verdict": "unfit", "reasons": ["soil_waterlogged"], "area_above_limit_pct": 2}
    ]}
  ],
  "releases": [
    {"method": "airblast", "required_state": "ideal", "date": "2026-11-27"},
    {"method": "knapsack", "required_state": "moist", "date": null}
  ],
  "data_issues": ["terrain_not_calculated"]
}`

func TestFieldTrafficabilityEndpoint(t *testing.T) {
	stub := &conditionsStub{traffic: trafficabilityView(t)}
	e := newConditionsEnv(t, stub)
	path := "/farms/" + farmA1 + "/fields/" + terrainFieldID + "/trafficability"

	got := e.call(operatorAID, http.MethodGet, path+"?days=3", nil)
	if got.status != http.StatusOK {
		t.Fatalf("status %d: %s", got.status, got.raw)
	}
	sameJSON(t, got.raw, trafficabilityJSON)
	if in := stub.trafficIn; in.Days != 3 || in.FieldID != terrainFieldID || in.FarmID != farmA1 || string(in.Actor.UserID) != operatorAID {
		t.Errorf("input = %+v", in)
	}
	if got.header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", got.header.Get("Cache-Control"))
	}
	e.call(operatorAID, http.MethodGet, path, nil)
	if stub.trafficIn.Days != 0 {
		t.Errorf("without days the use case applies the default: %d", stub.trafficIn.Days)
	}
	for _, query := range []string{"?days=0", "?days=abc"} {
		if got := e.call(operatorAID, http.MethodGet, path+query, nil); got.status != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", query, got.status)
		}
	}
	stub.err = domain.Invalid("days", "deve estar entre 1 e 7")
	if got := e.call(operatorAID, http.MethodGet, path+"?days=9", nil); got.status != http.StatusUnprocessableEntity ||
		!slices.Equal(got.errorFields(t), []string{"days"}) {
		t.Errorf("days above 7: %d %s", got.status, got.raw)
	}
	stub.err = usecase.ErrNotFound
	if got := e.call(adminBID, http.MethodGet, path, nil); got.status != http.StatusNotFound {
		t.Errorf("not found: %d", got.status)
	}
}
