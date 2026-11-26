package postgres

import (
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
)

// Geometry travels between Go and PostGIS as GeoJSON text (ST_GeomFromGeoJSON /
// ST_AsGeoJSON), so the adapter needs no geometry library. An empty string
// means "no geometry".

type geoJSONGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// perimeterGeoJSON encodes a perimeter as a MultiPolygon; nil gives a NULL parameter.
func perimeterGeoJSON(perimeter *domain.Perimeter) (pgtype.Text, error) {
	if perimeter == nil {
		return pgtype.Text{}, nil
	}
	text, err := encodePerimeter(*perimeter)
	if err != nil {
		return pgtype.Text{}, err
	}
	return pgtype.Text{String: text, Valid: true}, nil
}

func encodePerimeter(perimeter domain.Perimeter) (string, error) {
	coordinates, err := json.Marshal(perimeter)
	if err != nil {
		return "", fmt.Errorf("encode perimeter: %w", err)
	}
	body, err := json.Marshal(geoJSONGeometry{Type: "MultiPolygon", Coordinates: coordinates})
	if err != nil {
		return "", fmt.Errorf("encode perimeter: %w", err)
	}
	return string(body), nil
}

// pointGeoJSON encodes a point; nil gives a NULL parameter.
func pointGeoJSON(point *domain.GeoPoint) (pgtype.Text, error) {
	if point == nil {
		return pgtype.Text{}, nil
	}
	coordinates, err := json.Marshal([2]float64{point.Lng, point.Lat})
	if err != nil {
		return pgtype.Text{}, fmt.Errorf("encode point: %w", err)
	}
	body, err := json.Marshal(geoJSONGeometry{Type: "Point", Coordinates: coordinates})
	if err != nil {
		return pgtype.Text{}, fmt.Errorf("encode point: %w", err)
	}
	return pgtype.Text{String: string(body), Valid: true}, nil
}

// decodePerimeter reads a Polygon or MultiPolygon returned by ST_AsGeoJSON.
func decodePerimeter(text string) (*domain.Perimeter, error) {
	if text == "" {
		return nil, nil
	}
	var geometry geoJSONGeometry
	if err := json.Unmarshal([]byte(text), &geometry); err != nil {
		return nil, fmt.Errorf("decode perimeter: %w", err)
	}
	var polygons []domain.Polygon
	switch geometry.Type {
	case "MultiPolygon":
		if err := json.Unmarshal(geometry.Coordinates, &polygons); err != nil {
			return nil, fmt.Errorf("decode multipolygon: %w", err)
		}
	case "Polygon":
		var polygon domain.Polygon
		if err := json.Unmarshal(geometry.Coordinates, &polygon); err != nil {
			return nil, fmt.Errorf("decode polygon: %w", err)
		}
		polygons = []domain.Polygon{polygon}
	default:
		// Simplification of a tiny polygon may collapse it; the thumbnail is then omitted.
		return nil, nil
	}
	perimeter := domain.Perimeter(polygons)
	return &perimeter, nil
}

// decodePoint reads a Point returned by ST_AsGeoJSON.
func decodePoint(text string) (*domain.GeoPoint, error) {
	if text == "" {
		return nil, nil
	}
	var geometry geoJSONGeometry
	if err := json.Unmarshal([]byte(text), &geometry); err != nil {
		return nil, fmt.Errorf("decode point: %w", err)
	}
	var position [2]float64
	if err := json.Unmarshal(geometry.Coordinates, &position); err != nil {
		return nil, fmt.Errorf("decode point: %w", err)
	}
	return &domain.GeoPoint{Lat: position[1], Lng: position[0]}, nil
}
