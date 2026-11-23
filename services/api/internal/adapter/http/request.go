package httpadapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

const (
	// maxBodyBytes bounds every JSON body: the largest valid input is a few hundred bytes.
	maxBodyBytes = 1 << 20
	// maxSearchLength bounds the free text filter.
	maxSearchLength = 200
	maxEnumLength   = 32
)

// decodeJSON reads one JSON object from the body into dst. It is strict: the
// Content-Type must be application/json, the size is capped, unknown fields
// and trailing data are rejected. Every failure is an *apiError.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return &apiError{Status: http.StatusUnsupportedMediaType, Code: codeUnsupportedMediaType, Message: "envie o corpo como application/json"}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return bodyFailure(err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return bodyFailure(err)
		}
		return validationError("o corpo deve conter um único objeto JSON")
	}
	return nil
}

func bodyFailure(err error) error {
	var (
		tooLarge  *http.MaxBytesError
		typeError *json.UnmarshalTypeError
		syntax    *json.SyntaxError
	)
	switch {
	case errors.As(err, &tooLarge):
		return &apiError{Status: http.StatusRequestEntityTooLarge, Code: codePayloadTooLarge, Message: "corpo da requisição grande demais"}
	case errors.As(err, &typeError):
		if typeError.Field == "" {
			return validationError("o corpo deve ser um objeto JSON")
		}
		return validationError("dados inválidos", fieldDetail{Field: typeError.Field, Message: "tipo de valor inválido"})
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		name := strings.Trim(strings.TrimPrefix(err.Error(), "json: unknown field "), `"`)
		return validationError("dados inválidos", fieldDetail{Field: name, Message: "campo desconhecido"})
	case errors.Is(err, io.EOF):
		return validationError("o corpo da requisição está vazio")
	case errors.As(err, &syntax), errors.Is(err, io.ErrUnexpectedEOF):
		return validationError("o corpo da requisição não é um JSON válido")
	default:
		return validationError("o corpo da requisição não pôde ser lido")
	}
}

// query collects the problems of the query string so the client gets one 422
// listing every invalid parameter.
type query struct {
	values   url.Values
	problems domain.Violations
}

func newQuery(r *http.Request) *query { return &query{values: r.URL.Query()} }

// text returns the trimmed value of a free text parameter.
func (q *query) text(name string, maxLength int) string {
	value := strings.TrimSpace(q.values.Get(name))
	if utf8.RuneCountInString(value) > maxLength {
		q.problems.Add(name, "valor longo demais")
		return ""
	}
	return value
}

// enum returns the raw value of an enumerated parameter; the use case checks
// that it is a known one.
func (q *query) enum(name string) string { return q.text(name, maxEnumLength) }

// positiveInt returns 0 when the parameter is absent (the use case applies the default).
func (q *query) positiveInt(name string) int {
	raw := q.values.Get(name)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		q.problems.Add(name, "deve ser um inteiro maior ou igual a 1")
		return 0
	}
	return value
}

// uuid returns the canonical form of an id parameter, or "" when absent.
func (q *query) uuid(name string) string {
	raw := q.values.Get(name)
	if raw == "" {
		return ""
	}
	id, ok := canonicalUUID(raw)
	if !ok {
		q.problems.Add(name, "identificador inválido")
		return ""
	}
	return id
}

func (q *query) err() error { return q.problems.Err() }

// pathID reads a UUID path parameter. A malformed id cannot exist, so it is
// answered like any other missing resource.
func pathID(r *http.Request, name string) (string, error) {
	id, ok := canonicalUUID(r.PathValue(name))
	if !ok {
		return "", fmt.Errorf("%s %q: %w", name, r.PathValue(name), usecase.ErrNotFound)
	}
	return id, nil
}

// canonicalUUID accepts the 8-4-4-4-12 hexadecimal form and returns it in
// lower case, the form the database returns, so ids compare equal.
func canonicalUUID(value string) (string, bool) {
	if len(value) != 36 {
		return "", false
	}
	for i := range len(value) {
		char := value[i]
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if char != '-' {
				return "", false
			}
		case !isHex(char):
			return "", false
		}
	}
	return strings.ToLower(value), true
}

func isHex(char byte) bool {
	return char >= '0' && char <= '9' || char >= 'a' && char <= 'f' || char >= 'A' && char <= 'F'
}

// ---- endpoints -----------------------------------------------------------------------

// response is what an endpoint produces on success. A nil body sends no content.
type response struct {
	status int
	body   any
	header map[string]string
}

// endpoint is a handler body: it receives the authenticated principal and
// returns a response or an error. Errors are translated in one place.
type endpoint func(w http.ResponseWriter, r *http.Request, actor domain.Principal) (response, error)

// serve adapts an endpoint to http.HandlerFunc.
func serve(logger *slog.Logger, fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, ok := PrincipalFromContext(r.Context())
		if !ok {
			// A protected route reached without RequireAuth is a wiring bug: fail closed.
			writeFailure(w, r, logger, usecase.ErrUnauthenticated)
			return
		}
		result, err := fn(w, r, actor)
		if err != nil {
			writeFailure(w, r, logger, err)
			return
		}
		for name, value := range result.header {
			w.Header().Set(name, value)
		}
		if result.body == nil {
			w.WriteHeader(result.status)
			return
		}
		writeJSON(w, logger, result.status, result.body)
	}
}

func okResponse(body any) response      { return response{status: http.StatusOK, body: body} }
func createdResponse(body any) response { return response{status: http.StatusCreated, body: body} }
func noContentResponse() response       { return response{status: http.StatusNoContent} }
