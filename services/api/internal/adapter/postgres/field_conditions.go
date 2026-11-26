package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/adapter/postgres/sqlcgen"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Rain gauges, readings, soil inspections, soil profiles and the thresholds
// table (DER: rain_gauges, rain_gauge_fields, rain_readings, soil_inspections,
// trafficability_thresholds; ADR 0022, addendum). Only data: the soil state is
// derived in the domain.

// gaugeColumns is the shape shared by the gauge reads.
type gaugeColumns struct {
	ID, CompanyID, FarmID, Name string
	HasLocation                 bool
	Latitude, Longitude         float64
	CreatedAt, UpdatedAt        time.Time
	Version                     int32
}

func rainGaugeOf(c gaugeColumns) domain.RainGauge {
	gauge := domain.RainGauge{
		ID: domain.RainGaugeID(c.ID), CompanyID: domain.CompanyID(c.CompanyID), FarmID: domain.FarmID(c.FarmID),
		Name: c.Name, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Version: int(c.Version),
	}
	if c.HasLocation {
		gauge.Location = &domain.GeoPoint{Lat: c.Latitude, Lng: c.Longitude}
	}
	return gauge
}

func locationParams(location *domain.GeoPoint) (latitude, longitude pgtype.Float8) {
	if location == nil {
		return pgtype.Float8{}, pgtype.Float8{}
	}
	return pgtype.Float8{Float64: location.Lat, Valid: true}, pgtype.Float8{Float64: location.Lng, Valid: true}
}

// ---- reading -------------------------------------------------------------------------

// ListRainGauges implements usecase.RainGaugeReader.
func (r *Repository) ListRainGauges(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) ([]domain.RainGauge, error) {
	gauges := []domain.RainGauge{}
	if !validUUID(string(companyID)) || !validUUID(string(farmID)) {
		return gauges, nil
	}
	rows, err := r.q.ListRainGauges(ctx, sqlcgen.ListRainGaugesParams{CompanyID: string(companyID), FarmID: string(farmID)})
	if err != nil {
		return nil, fmt.Errorf("list rain gauges: %w", err)
	}
	for _, row := range rows {
		gauges = append(gauges, rainGaugeOf(gaugeColumns(row)))
	}
	return gauges, nil
}

// FindRainGauge implements usecase.RainGaugeReader.
func (r *Repository) FindRainGauge(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.RainGaugeID) (domain.RainGauge, error) {
	row, err := r.q.GetRainGauge(ctx, sqlcgen.GetRainGaugeParams{CompanyID: string(companyID), FarmID: string(farmID), ID: string(id)})
	if err != nil {
		return domain.RainGauge{}, translateRead("find rain gauge", err)
	}
	return rainGaugeOf(gaugeColumns(row)), nil
}

// ListRainReadings implements usecase.RainGaugeReader.
func (r *Repository) ListRainReadings(ctx context.Context, companyID domain.CompanyID, gaugeID domain.RainGaugeID, after, until time.Time) ([]usecase.StoredRainReading, error) {
	readings := []usecase.StoredRainReading{}
	if !validUUID(string(companyID)) || !validUUID(string(gaugeID)) {
		return readings, nil
	}
	rows, err := r.q.ListRainReadings(ctx, sqlcgen.ListRainReadingsParams{
		CompanyID: string(companyID), RainGaugeID: string(gaugeID), ObservedAfter: after, ObservedUntil: until,
	})
	if err != nil {
		return nil, fmt.Errorf("list rain readings: %w", err)
	}
	for _, row := range rows {
		readings = append(readings, usecase.StoredRainReading{
			Reading: domain.RainReading{
				ID: domain.RainReadingID(row.ID), CompanyID: domain.CompanyID(row.CompanyID), GaugeID: domain.RainGaugeID(row.RainGaugeID),
				RainMm: row.RainMm, ObservedAt: row.ObservedAt, RecordedBy: domain.UserID(row.RecordedBy), RecordedAt: row.RecordedAt,
			},
			RecordedByName: row.RecordedByName,
		})
	}
	return readings, nil
}

// ReadFieldConditions implements usecase.FieldConditionsReader.
func (r *Repository) ReadFieldConditions(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (usecase.FieldConditionsData, error) {
	row, err := r.q.GetFieldConditions(ctx, sqlcgen.GetFieldConditionsParams{CompanyID: string(companyID), FarmID: string(farmID), ID: string(fieldID)})
	if err != nil {
		return usecase.FieldConditionsData{}, translateRead("read field conditions", err)
	}
	data := usecase.FieldConditionsData{
		FieldID: domain.FieldID(row.ID), FarmID: domain.FarmID(row.FarmID),
		Centroid: domain.GeoPoint{Lat: row.CentroidLatitude, Lng: row.CentroidLongitude},
	}
	if row.SoilTexture.Valid {
		texture := domain.SoilTexture(row.SoilTexture.String)
		data.Texture = &texture
	}
	if row.GaugeID != nil && row.GaugeCreatedAt != nil && row.GaugeUpdatedAt != nil {
		gauge := rainGaugeOf(gaugeColumns{
			ID: *row.GaugeID, CompanyID: string(companyID), FarmID: row.FarmID, Name: row.GaugeName.String,
			HasLocation: row.GaugeHasLocation, Latitude: row.GaugeLatitude, Longitude: row.GaugeLongitude,
			CreatedAt: *row.GaugeCreatedAt, UpdatedAt: *row.GaugeUpdatedAt, Version: row.GaugeVersion.Int32,
		})
		data.Gauge = &gauge
	}
	return data, nil
}

// LatestSoilInspection implements usecase.FieldConditionsReader.
func (r *Repository) LatestSoilInspection(ctx context.Context, companyID domain.CompanyID, fieldID domain.FieldID) (*usecase.StoredSoilInspection, error) {
	if !validUUID(string(companyID)) || !validUUID(string(fieldID)) {
		return nil, nil
	}
	row, err := r.q.GetLatestSoilInspection(ctx, sqlcgen.GetLatestSoilInspectionParams{CompanyID: string(companyID), FieldID: string(fieldID)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("latest soil inspection: %w", err)
	}
	return &usecase.StoredSoilInspection{
		Inspection: domain.FieldSoilInspection{
			ID: domain.SoilInspectionID(row.ID), CompanyID: domain.CompanyID(row.CompanyID), FarmID: domain.FarmID(row.FarmID),
			FieldID: domain.FieldID(row.FieldID), State: domain.SoilState(row.State), InspectedAt: row.InspectedAt,
			InspectedBy: domain.UserID(row.InspectedBy), Note: textPtr(row.Note), RecordedAt: row.RecordedAt,
		},
		InspectedByName: row.InspectedByName,
	}, nil
}

// CurrentTrafficabilityThresholds implements usecase.TrafficabilityThresholdsReader.
func (r *Repository) CurrentTrafficabilityThresholds(ctx context.Context, on time.Time) (map[domain.SoilTexture]domain.TrafficabilityThresholds, error) {
	// The date of the local day, whatever the zone of `on`.
	day := time.Date(on.Year(), on.Month(), on.Day(), 0, 0, 0, 0, time.UTC)
	rows, err := r.q.ListCurrentTrafficabilityThresholds(ctx, pgtype.Date{Time: day, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list trafficability thresholds: %w", err)
	}
	thresholds := make(map[domain.SoilTexture]domain.TrafficabilityThresholds, len(rows))
	for _, row := range rows {
		thresholds[domain.SoilTexture(row.SoilTexture)] = domain.TrafficabilityThresholds{
			Rain48hUpperMm:       [3]float64{row.Rain48hDryMm, row.Rain48hIdealMm, row.Rain48hMoistMm},
			MoistureUpperPctOfFC: [3]float64{row.MoistureDryPct, row.MoistureIdealPct, row.MoistureMoistPct},
			Rain72hFactor:        row.Rain72hFactor,
			DryingRateMmPerDay:   row.DryingRateMmPerDay,
			FieldCapacityM3M3:    row.FieldCapacityM3m3,
			EffectiveFrom:        row.EffectiveFrom.Time,
			Source:               row.Source,
		}
	}
	return thresholds, nil
}

// ---- writing -------------------------------------------------------------------------

// LockFarmForConditions implements usecase.FieldConditionsStore.
func (r *Repository) LockFarmForConditions(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID) error {
	_, err := r.q.LockFarmForConditions(ctx, sqlcgen.LockFarmForConditionsParams{CompanyID: string(companyID), ID: string(farmID)})
	return translateRead("lock farm for conditions", err)
}

// InsertRainGauge implements usecase.FieldConditionsStore.
func (r *Repository) InsertRainGauge(ctx context.Context, gauge domain.RainGauge) error {
	latitude, longitude := locationParams(gauge.Location)
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		return q.InsertRainGauge(ctx, sqlcgen.InsertRainGaugeParams{
			ID: string(gauge.ID), CompanyID: string(gauge.CompanyID), FarmID: string(gauge.FarmID), Name: gauge.Name,
			Latitude: latitude, Longitude: longitude, CreatedAt: gauge.CreatedAt, UpdatedAt: gauge.UpdatedAt,
		})
	})
	return translateWrite("insert rain gauge", err)
}

// LockRainGauge implements usecase.FieldConditionsStore.
func (r *Repository) LockRainGauge(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, id domain.RainGaugeID) (domain.RainGauge, error) {
	row, err := r.q.LockRainGauge(ctx, sqlcgen.LockRainGaugeParams{CompanyID: string(companyID), FarmID: string(farmID), ID: string(id)})
	if err != nil {
		return domain.RainGauge{}, translateRead("lock rain gauge", err)
	}
	return rainGaugeOf(gaugeColumns(row)), nil
}

// UpdateRainGauge implements usecase.FieldConditionsStore.
func (r *Repository) UpdateRainGauge(ctx context.Context, gauge domain.RainGauge) error {
	latitude, longitude := locationParams(gauge.Location)
	var rows int64
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		var err error
		rows, err = q.UpdateRainGauge(ctx, sqlcgen.UpdateRainGaugeParams{
			Name: gauge.Name, Latitude: latitude, Longitude: longitude, UpdatedAt: gauge.UpdatedAt,
			CompanyID: string(gauge.CompanyID), FarmID: string(gauge.FarmID), ID: string(gauge.ID),
			Version: normalizeVersion(gauge.Version),
		})
		return err
	})
	if err != nil {
		return translateWrite("update rain gauge", err)
	}
	if rows == 0 {
		return fmt.Errorf("update rain gauge: %w", usecase.ErrVersionConflict)
	}
	return nil
}

// InsertRainReading implements usecase.FieldConditionsStore.
func (r *Repository) InsertRainReading(ctx context.Context, reading domain.RainReading) error {
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		return q.InsertRainReading(ctx, sqlcgen.InsertRainReadingParams{
			ID: string(reading.ID), CompanyID: string(reading.CompanyID), RainGaugeID: string(reading.GaugeID),
			RainMm: reading.RainMm, ObservedAt: reading.ObservedAt, RecordedBy: string(reading.RecordedBy), RecordedAt: reading.RecordedAt,
		})
	})
	return translateWrite("insert rain reading", err)
}

// LockFieldForConditions implements usecase.FieldConditionsStore.
func (r *Repository) LockFieldForConditions(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID) (domain.Field, error) {
	row, err := r.q.LockFieldForConditions(ctx, sqlcgen.LockFieldForConditionsParams{CompanyID: string(companyID), FarmID: string(farmID), ID: string(fieldID)})
	if err != nil {
		return domain.Field{}, translateRead("lock field for conditions", err)
	}
	return domain.Field{ID: domain.FieldID(row.ID), CompanyID: domain.CompanyID(row.CompanyID), FarmID: domain.FarmID(row.FarmID)}, nil
}

// SaveSoilProfile implements usecase.FieldConditionsStore: texture and link in
// one atomic step. The link FK uses the farm of the field, so a gauge of
// another farm is refused by the database too.
func (r *Repository) SaveSoilProfile(ctx context.Context, companyID domain.CompanyID, farmID domain.FarmID, fieldID domain.FieldID, profile domain.SoilProfile, at time.Time) error {
	texture := pgtype.Text{}
	if profile.Texture != nil {
		texture = pgtype.Text{String: string(*profile.Texture), Valid: true}
	}
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		rows, err := q.UpdateFieldSoilTexture(ctx, sqlcgen.UpdateFieldSoilTextureParams{
			SoilTexture: texture, CompanyID: string(companyID), FarmID: string(farmID), ID: string(fieldID),
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return fmt.Errorf("field: %w", usecase.ErrNotFound)
		}
		if profile.RainGaugeID == nil {
			return q.DeleteRainGaugeField(ctx, sqlcgen.DeleteRainGaugeFieldParams{CompanyID: string(companyID), FarmID: string(farmID), FieldID: string(fieldID)})
		}
		return q.UpsertRainGaugeField(ctx, sqlcgen.UpsertRainGaugeFieldParams{
			CompanyID: string(companyID), FarmID: string(farmID), FieldID: string(fieldID),
			RainGaugeID: string(*profile.RainGaugeID), LinkedAt: at,
		})
	})
	return translateWrite("save soil profile", err)
}

// InsertSoilInspection implements usecase.FieldConditionsStore.
func (r *Repository) InsertSoilInspection(ctx context.Context, inspection domain.FieldSoilInspection) error {
	err := r.atomic(ctx, func(q *sqlcgen.Queries) error {
		return q.InsertSoilInspection(ctx, sqlcgen.InsertSoilInspectionParams{
			ID: string(inspection.ID), CompanyID: string(inspection.CompanyID), FarmID: string(inspection.FarmID),
			FieldID: string(inspection.FieldID), State: string(inspection.State), InspectedAt: inspection.InspectedAt,
			InspectedBy: string(inspection.InspectedBy), Note: ptrText(inspection.Note), RecordedAt: inspection.RecordedAt,
		})
	})
	return translateWrite("insert soil inspection", err)
}
