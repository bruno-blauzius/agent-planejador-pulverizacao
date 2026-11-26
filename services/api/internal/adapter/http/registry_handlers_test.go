package httpadapter_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

const (
	ownerHelena = "00000000-0000-4000-8000-0000000000e1"
	ownerMarcos = "00000000-0000-4000-8000-0000000000e2"
	ownerOfB    = "00000000-0000-4000-8000-0000000000e3"
)

// seedRegistry gives company A the farm "Santa Rita" (Helena 60%) and two
// owners, and company B one owner.
func seedRegistry(e *apiEnv) {
	for id, owner := range map[string]struct{ company, name, document string }{
		ownerHelena: {companyAID, "Helena Prado", "52998224725"},
		ownerMarcos: {companyAID, "Marcos Prado", "11144477735"},
		ownerOfB:    {companyBID, "Bruna Lima", "52998224725"},
	} {
		e.reg.Owners[domain.OwnerID(id)] = domain.Owner{
			ID: domain.OwnerID(id), CompanyID: domain.CompanyID(owner.company), PersonType: domain.PersonIndividual,
			Name: owner.name, Document: domain.TaxID(owner.document), Version: 1, CreatedAt: e.clock.T, UpdatedAt: e.clock.T,
		}
	}
	sixty := domain.SharePercent(6000)
	e.reg.Farms[farmA1] = domain.Farm{
		ID: farmA1, CompanyID: companyAID, Name: "Santa Rita", ProductionType: domain.ProductionCoffee,
		City: "Guaxupé", State: "MG", Status: domain.FarmDraft, Version: 1, CreatedAt: e.clock.T, UpdatedAt: e.clock.T,
	}
	e.reg.Links = append(e.reg.Links, domain.Ownership{FarmID: farmA1, OwnerID: ownerHelena, Role: domain.LinkOwner, Share: &sixty})
}

func newFarmBody(mutate func(map[string]any)) map[string]any {
	body := map[string]any{
		"name": "Recanto", "code": "REC-01", "cooperative": nil, "production_type": "coffee",
		"city": "Guaxupé", "state": "mg", "status": "active",
		"location": map[string]any{
			"postal_code": "37800-000", "car_number": nil, "forecast_point": nil,
			"perimeter": map[string]any{
				"type": "Polygon", "bbox": []float64{-47, -21, -46.99, -20.99},
				"coordinates": [][][]float64{{{-47, -21}, {-47, -20.99}, {-46.99, -20.99}, {-46.99, -21}, {-47, -21}}},
			},
		},
		"owners": []map[string]any{
			{"owner_id": ownerHelena, "role": "owner", "share_percent": 60},
			{"owner_id": ownerMarcos, "role": "owner", "share_percent": 40},
		},
	}
	if mutate != nil {
		mutate(body)
	}
	return body
}

func TestListAndGetFarms(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)

	got := e.call(adminAID, http.MethodGet, "/farms", nil)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.raw)
	}
	body := got.object(t)
	items, _ := body["items"].([]any)
	if body["total"] != float64(1) || len(items) != 1 {
		t.Fatalf("farms of A: %s", got.raw)
	}
	farm, _ := items[0].(map[string]any)
	owners, _ := farm["owners"].([]any)
	first, _ := owners[0].(map[string]any)
	if farm["company_id"] != companyAID || farm["field_count"] != float64(0) || farm["spray_outlook"] != nil ||
		first["name"] != "Helena Prado" || first["share_percent"] != float64(60) {
		t.Errorf("farm summary shape: %v", farm)
	}
	if crops, ok := farm["crops"].([]any); !ok || len(crops) != 0 {
		t.Errorf("crops must be an empty list: %v", farm["crops"])
	}

	if other := e.call(adminBID, http.MethodGet, "/farms", nil); other.object(t)["total"] != float64(0) {
		t.Errorf("company B sees farms of A: %s", other.raw)
	}
	if bad := e.call(adminAID, http.MethodGet, "/farms?status=archived&production_type=forestry", nil); bad.status != http.StatusUnprocessableEntity {
		t.Errorf("bad filters: status %d", bad.status)
	}

	detail := e.call(operatorAID, http.MethodGet, "/farms/"+farmA1, nil)
	if detail.status != http.StatusOK || detail.object(t)["location"] == nil {
		t.Errorf("operator reads the farm: %d %s", detail.status, detail.raw)
	}
	for name, path := range map[string]string{"other company": "/farms/" + farmA1, "malformed id": "/farms/xyz"} {
		if missing := e.call(adminBID, http.MethodGet, path, nil); missing.status != http.StatusNotFound {
			t.Errorf("%s: status %d", name, missing.status)
		}
	}
}

func TestCreateAndUpdateFarm(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)

	created := e.call(adminAID, http.MethodPost, "/farms", newFarmBody(nil))
	if created.status != http.StatusCreated {
		t.Fatalf("status = %d, body %s", created.status, created.raw)
	}
	farm := created.object(t)
	location, _ := farm["location"].(map[string]any)
	perimeter, _ := location["perimeter"].(map[string]any)
	if farm["state"] != "MG" || farm["status"] != "active" || location["postal_code"] != "37800000" || perimeter["type"] != "Polygon" {
		t.Errorf("created farm: %s", created.raw)
	}
	id, _ := farm["id"].(string)

	updated := e.call(adminAID, http.MethodPut, "/farms/"+id, newFarmBody(func(b map[string]any) {
		b["name"], b["status"] = "Recanto Novo", "draft"
		b["owners"] = []map[string]any{{"owner_id": ownerMarcos, "role": "tenant", "share_percent": nil}}
	}))
	if updated.status != http.StatusOK || updated.object(t)["name"] != "Recanto Novo" {
		t.Fatalf("update: %d %s", updated.status, updated.raw)
	}
}

func TestCreateFarmRefusals(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)
	tests := []struct {
		name       string
		as         string
		body       any
		status     int
		wantFields []string
	}{
		{"operator cannot write", operatorAID, newFarmBody(nil), http.StatusForbidden, nil},
		{"active below 100%", adminAID, newFarmBody(func(b map[string]any) {
			b["owners"] = []map[string]any{{"owner_id": ownerHelena, "role": "owner", "share_percent": 50}}
		}), http.StatusUnprocessableEntity, []string{"owners"}},
		{"owner of another company", adminAID, newFarmBody(func(b map[string]any) {
			b["owners"] = []map[string]any{{"owner_id": ownerOfB, "role": "owner", "share_percent": 100}}
		}), http.StatusUnprocessableEntity, []string{"owners[0].owner_id"}},
		{"perimeter is not a polygon", adminAID, newFarmBody(func(b map[string]any) {
			b["location"].(map[string]any)["perimeter"] = map[string]any{"type": "Point", "coordinates": []float64{0, 0}}
		}), http.StatusUnprocessableEntity, []string{"location.perimeter"}},
		{"forecast point out of range", adminAID, newFarmBody(func(b map[string]any) {
			b["location"].(map[string]any)["forecast_point"] = map[string]any{"lat": 95, "lng": 0}
		}), http.StatusUnprocessableEntity, []string{"location.forecast_point"}},
		{"unknown field", adminAID, newFarmBody(func(b map[string]any) { b["owner"] = "x" }), http.StatusUnprocessableEntity, []string{"owner"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.call(tt.as, http.MethodPost, "/farms", tt.body)
			if got.status != tt.status {
				t.Fatalf("status = %d, want %d: %s", got.status, tt.status, got.raw)
			}
			if tt.wantFields != nil && !slices.Equal(got.errorFields(t), tt.wantFields) {
				t.Fatalf("fields = %v, want %v", got.errorFields(t), tt.wantFields)
			}
		})
	}
	plain := e.call(adminAID, http.MethodPost, "/farms", "name=x", withHeader("Content-Type", "text/plain"))
	if plain.status != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain: status %d", plain.status)
	}
}

func newOwnerBody(mutate func(map[string]any)) map[string]any {
	body := map[string]any{
		"person_type": "individual", "name": "Joana Lima", "document": "390.533.447-05",
		"birth_date": "1985-07-01", "phone": "(35) 99812-4470", "email": "Joana@Example.com", "window_alerts_enabled": true,
	}
	if mutate != nil {
		mutate(body)
	}
	return body
}

func TestOwnersEndpoints(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)

	created := e.call(adminAID, http.MethodPost, "/owners", newOwnerBody(nil))
	if created.status != http.StatusCreated {
		t.Fatalf("status = %d, body %s", created.status, created.raw)
	}
	owner := created.object(t)
	if owner["document"] != "39053344705" || owner["phone"] != "35998124470" || owner["email"] != "joana@example.com" ||
		owner["birth_date"] != "1985-07-01" || owner["has_pending_documents"] != true {
		t.Errorf("created owner: %s", created.raw)
	}
	if docs, ok := owner["documents"].([]any); !ok || len(docs) != 0 {
		t.Errorf("documents must be an empty list: %v", owner["documents"])
	}

	duplicate := e.call(adminAID, http.MethodPost, "/owners", newOwnerBody(nil))
	if duplicate.status != http.StatusConflict || duplicate.errorCode(t) != "conflict" {
		t.Errorf("duplicate document: %d %s", duplicate.status, duplicate.raw)
	}
	badDate := e.call(adminAID, http.MethodPost, "/owners", newOwnerBody(func(b map[string]any) { b["birth_date"] = "01/07/1985" }))
	if badDate.status != http.StatusUnprocessableEntity || !slices.Equal(badDate.errorFields(t), []string{"birth_date"}) {
		t.Errorf("bad date: %d %s", badDate.status, badDate.raw)
	}

	list := e.call(adminAID, http.MethodGet, "/owners?search=111.444", nil)
	if list.object(t)["total"] != float64(1) {
		t.Errorf("search by masked document: %s", list.raw)
	}
	if forbidden := e.call(operatorAID, http.MethodPut, "/owners/"+ownerHelena, newOwnerBody(nil)); forbidden.status != http.StatusForbidden {
		t.Errorf("operator update: status %d", forbidden.status)
	}
	if missing := e.call(adminBID, http.MethodGet, "/owners/"+ownerHelena, nil); missing.status != http.StatusNotFound {
		t.Errorf("owner of another company: status %d", missing.status)
	}

	renamed := e.call(adminAID, http.MethodPut, "/owners/"+ownerHelena, newOwnerBody(func(b map[string]any) {
		b["name"], b["document"] = "Helena P. Prado", "529.982.247-25"
	}))
	if renamed.status != http.StatusOK || renamed.object(t)["name"] != "Helena P. Prado" {
		t.Errorf("update: %d %s", renamed.status, renamed.raw)
	}
}

func TestOwnerLinksEndpoints(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)

	links := e.call(adminAID, http.MethodGet, "/owners/"+ownerHelena+"/links", nil)
	items, _ := links.object(t)["items"].([]any)
	if links.status != http.StatusOK || len(items) != 1 {
		t.Fatalf("links: %d %s", links.status, links.raw)
	}

	over := e.call(adminAID, http.MethodPut, "/owners/"+ownerMarcos+"/links", map[string]any{
		"links": []map[string]any{{"farm_id": farmA1, "role": "owner", "share_percent": 40.5}},
	})
	if over.status != http.StatusUnprocessableEntity || !slices.Equal(over.errorFields(t), []string{"links[0].share_percent"}) {
		t.Errorf("above 100%%: %d %s", over.status, over.raw)
	}

	replaced := e.call(adminAID, http.MethodPut, "/owners/"+ownerMarcos+"/links", map[string]any{
		"links": []map[string]any{{"farm_id": farmA1, "role": "owner", "share_percent": 40}},
	})
	items, _ = replaced.object(t)["items"].([]any)
	if replaced.status != http.StatusOK || len(items) != 1 {
		t.Fatalf("replace: %d %s", replaced.status, replaced.raw)
	}
	link, _ := items[0].(map[string]any)
	if link["farm_name"] != "Santa Rita" || link["share_percent"] != float64(40) {
		t.Errorf("link shape: %v", link)
	}
}

const uploadKML = `<kml><Document>
<Placemark><Polygon><outerBoundaryIs><LinearRing><coordinates>-47,-21 -47,-20.99 -46.99,-20.99 -46.99,-21 -47,-21</coordinates></LinearRing></outerBoundaryIs></Polygon></Placemark>
<Placemark><Polygon><outerBoundaryIs><LinearRing><coordinates>-46.999,-20.999 -46.999,-20.998 -46.998,-20.998 -46.999,-20.999</coordinates></LinearRing></outerBoundaryIs></Polygon></Placemark>
</Document></kml>`

func (e *apiEnv) upload(t *testing.T, as, field, fileName, content string) reply {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile(field, fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/perimeter-imports", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Test-User", as)
	recorder := httptest.NewRecorder()
	e.handler.ServeHTTP(recorder, request)
	return reply{status: recorder.Code, header: recorder.Header(), raw: recorder.Body.Bytes()}
}

func TestImportPerimeterEndpoint(t *testing.T) {
	e := newAPIEnv(t)

	got := e.upload(t, adminAID, "file", "fazenda.kml", uploadKML)
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.raw)
	}
	body := got.object(t)
	if body["source_format"] != "kml" || body["field_count"] != float64(1) || body["datum"] != "WGS 84" ||
		body["municipality"] != "Guaxupé" || body["state"] != "MG" || body["centroid"] == nil || body["area_ha"] == nil {
		t.Errorf("import shape: %s", got.raw)
	}

	if shapefile := e.upload(t, adminAID, "file", "fazenda.zip", "PK"); shapefile.status != http.StatusUnprocessableEntity {
		t.Errorf("broken shapefile: status %d %s", shapefile.status, shapefile.raw)
	}
	if other := e.upload(t, adminAID, "file", "fazenda.gdb", "x"); other.status != http.StatusUnsupportedMediaType {
		t.Errorf("unsupported format: status %d %s", other.status, other.raw)
	}
	if missing := e.upload(t, adminAID, "other", "fazenda.kml", uploadKML); missing.status != http.StatusUnprocessableEntity {
		t.Errorf("missing file part: status %d", missing.status)
	}
	if broken := e.upload(t, adminAID, "file", "fazenda.kml", "<kml"); broken.status != http.StatusUnprocessableEntity {
		t.Errorf("broken file: status %d", broken.status)
	}
	if forbidden := e.upload(t, operatorAID, "file", "fazenda.kml", uploadKML); forbidden.status != http.StatusForbidden {
		t.Errorf("operator: status %d", forbidden.status)
	}
	if notMultipart := e.call(adminAID, http.MethodPost, "/perimeter-imports", map[string]any{}); notMultipart.status != http.StatusUnsupportedMediaType {
		t.Errorf("json body: status %d", notMultipart.status)
	}
}

// ---- fields and crops ----------------------------------------------------------------

func fieldBody(name string, west, east float64) map[string]any {
	x0, x1 := -47+west/1000, -47+east/1000
	return map[string]any{
		"name": name, "block": "Área Norte", "crop_code": "coffee_arabica", "variety": "Catuaí 144",
		"season": "2026/27", "planting_date": "2024-10-01", "row_spacing_m": 3.5, "plant_spacing_m": 0.7,
		"polygon": map[string]any{
			"type":        "Polygon",
			"coordinates": [][][]float64{{{x0, -20.999}, {x0, -20.997}, {x1, -20.997}, {x1, -20.999}, {x0, -20.999}}},
		},
	}
}

func TestFieldsEndpoints(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)
	farm := e.reg.Farms[farmA1]
	perimeter := domain.Perimeter{{{{-47, -21}, {-47, -20.99}, {-46.99, -20.99}, {-46.99, -21}, {-47, -21}}}}
	farm.Perimeter = &perimeter
	e.reg.Farms[farmA1] = farm
	path := "/farms/" + farmA1 + "/fields"

	created := e.call(adminAID, http.MethodPost, path, fieldBody("Talhão 1", 1, 3))
	if created.status != http.StatusCreated {
		t.Fatalf("create status = %d, body %s", created.status, created.raw)
	}
	field := created.object(t)
	crop, _ := field["crop"].(map[string]any)
	polygon, _ := field["polygon"].(map[string]any)
	if field["farm_id"] != farmA1 || crop["name"] != "Café arábica" || field["plants_per_ha"] != float64(4082) ||
		field["planting_date"] != "2024-10-01" || polygon["type"] != "Polygon" || field["spray_condition"] != nil ||
		field["last_application_at"] != nil {
		t.Fatalf("field shape: %s", created.raw)
	}
	if area, _ := field["area_ha"].(float64); area <= 0 {
		t.Fatalf("area_ha = %v", field["area_ha"])
	}

	overlap := e.call(adminAID, http.MethodPost, path, fieldBody("Talhão 2", 2, 4))
	if overlap.status != http.StatusUnprocessableEntity || !slices.Contains(overlap.errorFields(t), "polygon") {
		t.Fatalf("overlap: %d %s", overlap.status, overlap.raw)
	}
	if forbidden := e.call(operatorAID, http.MethodPost, path, fieldBody("Talhão 3", 5, 6)); forbidden.status != http.StatusForbidden {
		t.Fatalf("operator create: %d", forbidden.status)
	}
	if hidden := e.call(adminBID, http.MethodGet, path, nil); hidden.status != http.StatusNotFound {
		t.Fatalf("other company list: %d", hidden.status)
	}

	id, _ := field["id"].(string)
	update := fieldBody("Talhão 1A", 1, 3)
	update["crop_code"], update["row_spacing_m"], update["plant_spacing_m"] = "soybean", nil, nil
	updated := e.call(adminAID, http.MethodPut, path+"/"+id, update)
	if updated.status != http.StatusOK || updated.object(t)["name"] != "Talhão 1A" || updated.object(t)["plants_per_ha"] != nil {
		t.Fatalf("update: %d %s", updated.status, updated.raw)
	}
	if missing := e.call(adminAID, http.MethodPut, path+"/"+ownerHelena, update); missing.status != http.StatusNotFound {
		t.Fatalf("unknown field: %d", missing.status)
	}

	list := e.call(operatorAID, http.MethodGet, path, nil)
	items, _ := list.object(t)["items"].([]any)
	if list.status != http.StatusOK || len(items) != 1 {
		t.Fatalf("list: %d %s", list.status, list.raw)
	}
	summary := e.call(adminAID, http.MethodGet, "/farms/"+farmA1, nil).object(t)
	crops, _ := summary["crops"].([]any)
	if summary["field_count"] != float64(1) || len(crops) != 1 {
		t.Fatalf("farm summary after fields: %v", summary)
	}
}

func TestFieldInputValidation(t *testing.T) {
	e := newAPIEnv(t)
	seedRegistry(e)
	body := fieldBody("", 1, 3)
	body["crop_code"], body["season"], body["planting_date"] = "banana", "26/27", "01/10/2024"
	body["polygon"] = map[string]any{"type": "Point", "coordinates": []float64{-47, -21}}
	got := e.call(adminAID, http.MethodPost, "/farms/"+farmA1+"/fields", body)
	if got.status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, body %s", got.status, got.raw)
	}
	fields := got.errorFields(t)
	for _, want := range []string{"planting_date", "polygon"} {
		if !slices.Contains(fields, want) {
			t.Errorf("missing %s in %v", want, fields)
		}
	}
}

func TestListCropsEndpoint(t *testing.T) {
	e := newAPIEnv(t)
	got := e.call(operatorAID, http.MethodGet, "/crops", nil)
	items, _ := got.object(t)["items"].([]any)
	if got.status != http.StatusOK || len(items) != 3 {
		t.Fatalf("crops: %d %s", got.status, got.raw)
	}
	first, _ := items[0].(map[string]any)
	if first["code"] != "coffee_arabica" || first["name"] != "Café arábica" {
		t.Fatalf("first crop: %v", first)
	}
}
