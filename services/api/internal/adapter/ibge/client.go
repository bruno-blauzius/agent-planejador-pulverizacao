// Package ibge reads the municipal registry of IBGE (Instituto Brasileiro de
// Geografia e Estatística) from its public data service: names and states
// from the localities API, boundaries from the mesh API (ADR 0019). It is used
// only by the loader command, never while serving requests.
package ibge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// DefaultBaseURL is the public data service of IBGE.
const DefaultBaseURL = "https://servicodados.ibge.gov.br"

// Mesh qualities offered by the API: the finer, the larger the download.
const (
	QualityMinimum      = "minima"
	QualityIntermediate = "intermediaria"
	QualityMaximum      = "maxima"
)

const (
	namesPath = "/api/v1/localidades/municipios"
	meshPath  = "/api/v3/malhas/paises/BR"
	// maxNamesBytes and maxMeshBytes bound the answers read into memory.
	maxNamesBytes = 32 << 20
	maxMeshBytes  = 512 << 20
)

// Client implements usecase.MunicipalitySource.
type Client struct {
	http    *http.Client
	baseURL string
	quality string
	onSkip  SkipFunc
}

// SkipFunc receives each municipality left out of the registry and why.
type SkipFunc func(code, name, reason string)

// NewClient builds the client. quality is one of the Quality constants;
// onSkip may be nil.
func NewClient(httpClient *http.Client, baseURL, quality string, onSkip SkipFunc) (*Client, error) {
	if onSkip == nil {
		onSkip = func(string, string, string) {}
	}
	switch quality {
	case QualityMinimum, QualityIntermediate, QualityMaximum:
	default:
		return nil, fmt.Errorf("unknown mesh quality %q (use %s, %s or %s)", quality, QualityMinimum, QualityIntermediate, QualityMaximum)
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("invalid IBGE base URL: %w", err)
	}
	return &Client{http: httpClient, baseURL: baseURL, quality: quality, onSkip: onSkip}, nil
}

var _ usecase.MunicipalitySource = (*Client)(nil)

// nameEntry is one item of the "nivelado" (flat) view of the localities API.
type nameEntry struct {
	ID    json.Number `json:"municipio-id"`
	Name  string      `json:"municipio-nome"`
	State string      `json:"UF-sigla"`
}

type meshCollection struct {
	Features []struct {
		Properties struct {
			Code string `json:"codarea"`
		} `json:"properties"`
		Geometry json.RawMessage `json:"geometry"`
	} `json:"features"`
}

// FetchMunicipalities joins names and boundaries by IBGE code. The two APIs
// are updated at different times: a municipality created after the last mesh
// (e.g. 5101837, Boa Esperança do Norte) has a name but no boundary, and one
// merged away may keep a boundary without a name. Those are left out and
// reported to onSkip; the use case still refuses a registry below its minimum.
func (c *Client) FetchMunicipalities(ctx context.Context) ([]usecase.MunicipalityBoundary, error) {
	var names []nameEntry
	if err := c.getJSON(ctx, namesPath, url.Values{"view": {"nivelado"}}, maxNamesBytes, &names); err != nil {
		return nil, fmt.Errorf("municipality names: %w", err)
	}
	var mesh meshCollection
	query := url.Values{
		"formato": {"application/vnd.geo+json"}, "intrarregiao": {"municipio"}, "qualidade": {c.quality},
	}
	if err := c.getJSON(ctx, meshPath, query, maxMeshBytes, &mesh); err != nil {
		return nil, fmt.Errorf("municipal mesh: %w", err)
	}
	boundaries := make(map[string]json.RawMessage, len(mesh.Features))
	for _, feature := range mesh.Features {
		boundaries[feature.Properties.Code] = feature.Geometry
	}
	municipalities := make([]usecase.MunicipalityBoundary, 0, len(names))
	for _, entry := range names {
		code := entry.ID.String()
		boundary, ok := boundaries[code]
		if !ok {
			c.onSkip(code, entry.Name, "no boundary in the mesh")
			continue
		}
		delete(boundaries, code)
		municipalities = append(municipalities, usecase.MunicipalityBoundary{
			Municipality: domain.Municipality{Code: code, Name: entry.Name, State: entry.State},
			Boundary:     string(boundary),
		})
	}
	for _, code := range slices.Sorted(maps.Keys(boundaries)) {
		c.onSkip(code, "", "boundary without a municipality name")
	}
	return municipalities, nil
}

func (c *Client) getJSON(ctx context.Context, path string, query url.Values, limit int64, target any) (err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "agentes-agro-api/municipality-loader")
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("request %s: %w", path, err)
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("request %s: status %s", path, strconv.Itoa(response.StatusCode))
	}
	body := io.LimitReader(response.Body, limit+1)
	counter := &countingReader{reader: body}
	if err := json.NewDecoder(counter).Decode(target); err != nil {
		if counter.read > limit {
			return fmt.Errorf("request %s: answer larger than %d bytes", path, limit)
		}
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

type countingReader struct {
	reader io.Reader
	read   int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.read += int64(n)
	return n, err
}
