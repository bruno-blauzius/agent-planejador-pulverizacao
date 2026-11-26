package httpadapter

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	// maxFarmBodyBytes bounds a farm body: the perimeter may carry thousands of vertices.
	maxFarmBodyBytes = 8 << 20
	// maxPerimeterFileBytes is the upload limit of POST /perimeter-imports (contract: 20 MB).
	maxPerimeterFileBytes = 20 << 20
	// multipartOverhead leaves room for the part headers around the file.
	multipartOverhead = 64 << 10
)

// FarmsHandler serves the Farms, Fields and Crops tags of the contract.
type FarmsHandler struct {
	logger *slog.Logger
	deps   FarmsDeps
}

// FarmsDeps groups the use cases of FarmsHandler.
type FarmsDeps struct {
	List            *usecase.ListFarms
	Get             *usecase.GetFarm
	Create          *usecase.CreateFarm
	Update          *usecase.UpdateFarm
	ImportPerimeter *usecase.ImportPerimeter
	ListFields      *usecase.ListFields
	CreateField     *usecase.CreateField
	UpdateField     *usecase.UpdateField
	ListCrops       *usecase.ListCrops
}

// NewFarmsHandler builds the handler.
func NewFarmsHandler(logger *slog.Logger, deps FarmsDeps) *FarmsHandler {
	return &FarmsHandler{logger: logger, deps: deps}
}

// Register attaches the routes (protected; registry.write is checked by the use cases).
func (h *FarmsHandler) Register(routes Routes) {
	routes.Protected(http.MethodGet, "/farms", serve(h.logger, h.listFarms))
	routes.Protected(http.MethodPost, "/farms", serve(h.logger, h.createFarm))
	routes.Protected(http.MethodGet, "/farms/{farmId}", serve(h.logger, h.getFarm))
	routes.Protected(http.MethodPut, "/farms/{farmId}", serve(h.logger, h.updateFarm))
	routes.Protected(http.MethodPost, "/perimeter-imports", serve(h.logger, h.importPerimeter))
	routes.Protected(http.MethodGet, "/farms/{farmId}/fields", serve(h.logger, h.listFields))
	routes.Protected(http.MethodPost, "/farms/{farmId}/fields", serve(h.logger, h.createField))
	routes.Protected(http.MethodPut, "/farms/{farmId}/fields/{fieldId}", serve(h.logger, h.updateField))
	routes.Protected(http.MethodGet, "/crops", serve(h.logger, h.listCrops))
}

func (h *FarmsHandler) listFarms(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	params := newQuery(r)
	in := usecase.ListFarmsInput{
		Actor:          actor,
		Search:         params.text("search", maxSearchLength),
		Status:         domain.FarmStatus(params.enum("status")),
		ProductionType: domain.ProductionType(params.enum("production_type")),
		Page:           params.positiveInt("page"),
		PageSize:       params.positiveInt("page_size"),
	}
	if err := params.err(); err != nil {
		return response{}, err
	}
	page, err := h.deps.List.Execute(r.Context(), in)
	if err != nil {
		return response{}, err
	}
	return okResponse(farmPageOf(page)), nil
}

func (h *FarmsHandler) getFarm(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "farmId")
	if err != nil {
		return response{}, err
	}
	listing, err := h.deps.Get.Execute(r.Context(), usecase.GetFarmInput{Actor: actor, FarmID: domain.FarmID(id)})
	if err != nil {
		return response{}, err
	}
	return okResponse(farmOf(listing)), nil
}

func readFarmDetails(w http.ResponseWriter, r *http.Request) (domain.FarmDetails, error) {
	var body farmInputBody
	if err := decodeJSONWithin(w, r, &body, maxFarmBodyBytes); err != nil {
		return domain.FarmDetails{}, err
	}
	return body.details()
}

func (h *FarmsHandler) createFarm(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	details, err := readFarmDetails(w, r)
	if err != nil {
		return response{}, err
	}
	listing, err := h.deps.Create.Execute(r.Context(), usecase.CreateFarmInput{Actor: actor, Details: details})
	if err != nil {
		return response{}, err
	}
	return createdResponse(farmOf(listing)), nil
}

func (h *FarmsHandler) updateFarm(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "farmId")
	if err != nil {
		return response{}, err
	}
	details, err := readFarmDetails(w, r)
	if err != nil {
		return response{}, err
	}
	listing, err := h.deps.Update.Execute(r.Context(), usecase.UpdateFarmInput{Actor: actor, FarmID: domain.FarmID(id), Details: details})
	if err != nil {
		return response{}, err
	}
	return okResponse(farmOf(listing)), nil
}

func (h *FarmsHandler) importPerimeter(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	file, err := readUploadedFile(w, r, "file", maxPerimeterFileBytes)
	if err != nil {
		return response{}, err
	}
	result, err := h.deps.ImportPerimeter.Execute(r.Context(), usecase.ImportPerimeterInput{Actor: actor, File: file})
	if err != nil {
		return response{}, err
	}
	return okResponse(perimeterImportOf(result)), nil
}

func (h *FarmsHandler) listFields(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	farmID, err := pathID(r, "farmId")
	if err != nil {
		return response{}, err
	}
	fields, err := h.deps.ListFields.Execute(r.Context(), usecase.ListFieldsInput{Actor: actor, FarmID: domain.FarmID(farmID)})
	if err != nil {
		return response{}, err
	}
	return okResponse(fieldListBody{Items: fieldsOf(fields)}), nil
}

func readFieldInput(w http.ResponseWriter, r *http.Request, actor domain.Principal) (usecase.SaveFieldInput, error) {
	farmID, err := pathID(r, "farmId")
	if err != nil {
		return usecase.SaveFieldInput{}, err
	}
	var body fieldInputBody
	if err := decodeJSONWithin(w, r, &body, maxFarmBodyBytes); err != nil {
		return usecase.SaveFieldInput{}, err
	}
	details, err := body.details()
	if err != nil {
		return usecase.SaveFieldInput{}, err
	}
	return usecase.SaveFieldInput{Actor: actor, FarmID: domain.FarmID(farmID), Details: details}, nil
}

func (h *FarmsHandler) createField(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	in, err := readFieldInput(w, r, actor)
	if err != nil {
		return response{}, err
	}
	listing, err := h.deps.CreateField.Execute(r.Context(), in)
	if err != nil {
		return response{}, err
	}
	return createdResponse(fieldOf(listing)), nil
}

func (h *FarmsHandler) updateField(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	fieldID, err := pathID(r, "fieldId")
	if err != nil {
		return response{}, err
	}
	in, err := readFieldInput(w, r, actor)
	if err != nil {
		return response{}, err
	}
	in.FieldID = domain.FieldID(fieldID)
	listing, err := h.deps.UpdateField.Execute(r.Context(), in)
	if err != nil {
		return response{}, err
	}
	return okResponse(fieldOf(listing)), nil
}

func (h *FarmsHandler) listCrops(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	crops, err := h.deps.ListCrops.Execute(r.Context(), actor)
	if err != nil {
		return response{}, err
	}
	return okResponse(cropListBody{Items: cropsOf(crops)}), nil
}

// readUploadedFile streams the multipart body and returns the part named
// field, refusing more than limit bytes (413) and other media types (415).
func readUploadedFile(w http.ResponseWriter, r *http.Request, field string, limit int64) (usecase.PerimeterFile, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return usecase.PerimeterFile{}, &apiError{Status: http.StatusUnsupportedMediaType, Code: codeUnsupportedMediaType, Message: "envie o arquivo como multipart/form-data"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit+multipartOverhead)
	reader, err := r.MultipartReader()
	if err != nil {
		return usecase.PerimeterFile{}, validationError("corpo multipart inválido")
	}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			return usecase.PerimeterFile{}, validationError("dados inválidos", fieldDetail{Field: field, Message: "envie o arquivo"})
		}
		if err != nil {
			return usecase.PerimeterFile{}, uploadFailure(err)
		}
		if part.FormName() != field {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(part, limit+1))
		if err != nil {
			return usecase.PerimeterFile{}, uploadFailure(err)
		}
		if int64(len(content)) > limit {
			return usecase.PerimeterFile{}, &apiError{Status: http.StatusRequestEntityTooLarge, Code: codePayloadTooLarge, Message: "arquivo maior que 20 MB"}
		}
		return usecase.PerimeterFile{Name: path.Base(part.FileName()), Content: content}, nil
	}
}

func uploadFailure(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return &apiError{Status: http.StatusRequestEntityTooLarge, Code: codePayloadTooLarge, Message: "arquivo maior que 20 MB"}
	}
	return validationError("corpo multipart inválido")
}

// OwnersHandler serves the Owners tag of the contract.
type OwnersHandler struct {
	logger       *slog.Logger
	list         *usecase.ListOwners
	get          *usecase.GetOwner
	create       *usecase.CreateOwner
	update       *usecase.UpdateOwner
	replaceLinks *usecase.ReplaceOwnerLinks
}

// OwnersDeps groups the use cases of OwnersHandler.
type OwnersDeps struct {
	List         *usecase.ListOwners
	Get          *usecase.GetOwner
	Create       *usecase.CreateOwner
	Update       *usecase.UpdateOwner
	ReplaceLinks *usecase.ReplaceOwnerLinks
}

// NewOwnersHandler builds the handler.
func NewOwnersHandler(logger *slog.Logger, deps OwnersDeps) *OwnersHandler {
	return &OwnersHandler{
		logger: logger, list: deps.List, get: deps.Get, create: deps.Create, update: deps.Update, replaceLinks: deps.ReplaceLinks,
	}
}

// Register attaches the routes (protected; registry.write is checked by the use cases).
func (h *OwnersHandler) Register(routes Routes) {
	routes.Protected(http.MethodGet, "/owners", serve(h.logger, h.listOwners))
	routes.Protected(http.MethodPost, "/owners", serve(h.logger, h.createOwner))
	routes.Protected(http.MethodGet, "/owners/{ownerId}", serve(h.logger, h.getOwner))
	routes.Protected(http.MethodPut, "/owners/{ownerId}", serve(h.logger, h.updateOwner))
	routes.Protected(http.MethodGet, "/owners/{ownerId}/links", serve(h.logger, h.getOwnerLinks))
	routes.Protected(http.MethodPut, "/owners/{ownerId}/links", serve(h.logger, h.putOwnerLinks))
}

func (h *OwnersHandler) listOwners(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	params := newQuery(r)
	in := usecase.ListOwnersInput{
		Actor:      actor,
		Search:     params.text("search", maxSearchLength),
		PersonType: domain.PersonType(params.enum("person_type")),
		Page:       params.positiveInt("page"),
		PageSize:   params.positiveInt("page_size"),
	}
	if err := params.err(); err != nil {
		return response{}, err
	}
	page, err := h.list.Execute(r.Context(), in)
	if err != nil {
		return response{}, err
	}
	return okResponse(ownerPageOf(page)), nil
}

func (h *OwnersHandler) findOwner(r *http.Request, actor domain.Principal) (usecase.OwnerListing, error) {
	id, err := pathID(r, "ownerId")
	if err != nil {
		return usecase.OwnerListing{}, err
	}
	return h.get.Execute(r.Context(), usecase.GetOwnerInput{Actor: actor, OwnerID: domain.OwnerID(id)})
}

func (h *OwnersHandler) getOwner(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	listing, err := h.findOwner(r, actor)
	if err != nil {
		return response{}, err
	}
	return okResponse(ownerOf(listing)), nil
}

func (h *OwnersHandler) getOwnerLinks(_ http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	listing, err := h.findOwner(r, actor)
	if err != nil {
		return response{}, err
	}
	return okResponse(ownerLinkListBody{Items: ownerLinksOf(listing.Links)}), nil
}

func readOwnerDetails(w http.ResponseWriter, r *http.Request) (domain.OwnerDetails, error) {
	var body ownerInputBody
	if err := decodeJSON(w, r, &body); err != nil {
		return domain.OwnerDetails{}, err
	}
	return body.details()
}

func (h *OwnersHandler) createOwner(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	details, err := readOwnerDetails(w, r)
	if err != nil {
		return response{}, err
	}
	listing, err := h.create.Execute(r.Context(), usecase.SaveOwnerInput{Actor: actor, Details: details})
	if err != nil {
		return response{}, err
	}
	return createdResponse(ownerOf(listing)), nil
}

func (h *OwnersHandler) updateOwner(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "ownerId")
	if err != nil {
		return response{}, err
	}
	details, err := readOwnerDetails(w, r)
	if err != nil {
		return response{}, err
	}
	listing, err := h.update.Execute(r.Context(), usecase.SaveOwnerInput{Actor: actor, OwnerID: domain.OwnerID(id), Details: details})
	if err != nil {
		return response{}, err
	}
	return okResponse(ownerOf(listing)), nil
}

func (h *OwnersHandler) putOwnerLinks(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error) {
	id, err := pathID(r, "ownerId")
	if err != nil {
		return response{}, err
	}
	var body ownerLinksBody
	if err := decodeJSON(w, r, &body); err != nil {
		return response{}, err
	}
	links, err := h.replaceLinks.Execute(r.Context(), usecase.ReplaceOwnerLinksInput{
		Actor: actor, OwnerID: domain.OwnerID(id), Links: body.inputs(),
	})
	if err != nil {
		return response{}, err
	}
	return okResponse(ownerLinkListBody{Items: ownerLinksOf(links)}), nil
}
