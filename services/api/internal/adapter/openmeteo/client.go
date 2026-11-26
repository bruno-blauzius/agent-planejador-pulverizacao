// Package openmeteo reads the Open-Meteo weather model (ADR 0022, addendum
// item 8): the hourly rain and soil moisture of the last 7 days and of the
// forecast, and the daily rain per local day, for one rounded coordinate.
// The client only translates the answer; the soil state is decided in the domain.
package openmeteo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/domain"
	"github.com/bruno-blauzius/agent-planejador-pulverizacao/services/api/internal/usecase"
)

// Defaults of Config.
const (
	DefaultBaseURL        = "https://api.open-meteo.com"
	DefaultRequestTimeout = 10 * time.Second
	DefaultMaxBytes       = int64(1 << 20)
	DefaultCacheTTL       = time.Hour
	DefaultUserAgent      = "agentes-agro-api"

	forecastPath = "/v1/forecast"
	// pastDays covers the 72 h and 7 day sums and the 7 past days of rain_daily.
	pastDays = 7
	// forecastDays is today plus the longest projection of the contract (7 days).
	forecastDays = 8
)

// soilLayers are the model layers from 0 to 27 cm and their thickness in cm:
// the moisture of the top 27 cm is their mean weighted by thickness.
var soilLayers = []struct {
	variable    string
	thicknessCm float64
}{
	{"soil_moisture_0_to_1cm", 1},
	{"soil_moisture_1_to_3cm", 2},
	{"soil_moisture_3_to_9cm", 6},
	{"soil_moisture_9_to_27cm", 18},
}

// Config holds the client settings; zero values take the defaults except
// Location, which is required (the local days are counted in it).
type Config struct {
	BaseURL        string
	RequestTimeout time.Duration
	MaxBytes       int64
	Location       *time.Location
	UserAgent      string
	// Logger receives the cause of failures; coordinates are never logged.
	Logger *slog.Logger
}

func (c Config) withDefaults() Config {
	if c.BaseURL == "" {
		c.BaseURL = DefaultBaseURL
	}
	c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	if c.RequestTimeout <= 0 {
		c.RequestTimeout = DefaultRequestTimeout
	}
	if c.MaxBytes <= 0 {
		c.MaxBytes = DefaultMaxBytes
	}
	if c.UserAgent == "" {
		c.UserAgent = DefaultUserAgent
	}
	if c.Logger == nil {
		c.Logger = slog.New(slog.DiscardHandler)
	}
	return c
}

// Client implements usecase.WeatherProvider over the Open-Meteo forecast API.
type Client struct {
	http *http.Client
	cfg  Config
}

// NewClient builds the client; a nil httpClient uses http.DefaultClient.
func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	if cfg.Location == nil {
		return nil, errors.New("openmeteo: a time zone is required")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{http: httpClient, cfg: cfg.withDefaults()}, nil
}

// Weather implements usecase.WeatherProvider. Every failure (network, status,
// size, malformed answer) wraps usecase.ErrWeatherUnavailable: the caller
// reports it and never invents a value.
func (c *Client) Weather(ctx context.Context, cell domain.WeatherCell) (usecase.WeatherReport, error) {
	report, err := c.fetch(ctx, cell)
	if err != nil {
		// The cause names no URL nor coordinate: the cell is company data sent to a third party.
		c.cfg.Logger.WarnContext(ctx, "weather provider unavailable", "provider", "open-meteo", "error", err)
		return usecase.WeatherReport{}, fmt.Errorf("open-meteo: %w: %w", usecase.ErrWeatherUnavailable, err)
	}
	return report, nil
}

func (c *Client) fetch(ctx context.Context, cell domain.WeatherCell) (usecase.WeatherReport, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.requestURL(cell), nil)
	if err != nil {
		return usecase.WeatherReport{}, errors.New("build request")
	}
	request.Header.Set("User-Agent", c.cfg.UserAgent)
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		return usecase.WeatherReport{}, fmt.Errorf("request failed: %w", unwrapURLError(err))
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
		return usecase.WeatherReport{}, fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, c.cfg.MaxBytes+1))
	if err != nil {
		return usecase.WeatherReport{}, fmt.Errorf("read body: %w", unwrapURLError(err))
	}
	if int64(len(body)) > c.cfg.MaxBytes {
		return usecase.WeatherReport{}, fmt.Errorf("answer larger than %d bytes", c.cfg.MaxBytes)
	}
	var answer forecastAnswer
	if err := json.Unmarshal(body, &answer); err != nil {
		return usecase.WeatherReport{}, errors.New("malformed answer")
	}
	return answer.report(c.cfg.Location)
}

// requestURL asks for exactly what the soil state needs. The coordinate goes
// with two decimals: it was already rounded to the 0.01 degree grid.
func (c *Client) requestURL(cell domain.WeatherCell) string {
	hourly := []string{"precipitation"}
	for _, layer := range soilLayers {
		hourly = append(hourly, layer.variable)
	}
	query := url.Values{}
	query.Set("latitude", strconv.FormatFloat(cell.Latitude, 'f', 2, 64))
	query.Set("longitude", strconv.FormatFloat(cell.Longitude, 'f', 2, 64))
	query.Set("hourly", strings.Join(hourly, ","))
	query.Set("daily", "precipitation_sum")
	query.Set("past_days", strconv.Itoa(pastDays))
	query.Set("forecast_days", strconv.Itoa(forecastDays))
	query.Set("timezone", c.cfg.Location.String())
	query.Set("timeformat", "unixtime")
	return c.cfg.BaseURL + forecastPath + "?" + query.Encode()
}

// forecastAnswer is the part of the Open-Meteo answer the client reads. With
// timeformat=unixtime every time is a Unix timestamp; daily times are the
// local midnights of the requested time zone.
type forecastAnswer struct {
	Hourly map[string]json.RawMessage `json:"hourly"`
	Daily  struct {
		Time             []int64    `json:"time"`
		PrecipitationSum []*float64 `json:"precipitation_sum"`
	} `json:"daily"`
}

func (a forecastAnswer) report(loc *time.Location) (usecase.WeatherReport, error) {
	var times []int64
	if err := decodeSeries(a.Hourly, "time", &times); err != nil {
		return usecase.WeatherReport{}, err
	}
	if len(times) == 0 {
		return usecase.WeatherReport{}, errors.New("answer without hourly data")
	}
	var rain []*float64
	if err := decodeSeries(a.Hourly, "precipitation", &rain); err != nil {
		return usecase.WeatherReport{}, err
	}
	layers := make([][]*float64, len(soilLayers))
	for i, layer := range soilLayers {
		if err := decodeSeries(a.Hourly, layer.variable, &layers[i]); err != nil {
			return usecase.WeatherReport{}, err
		}
	}
	if len(rain) != len(times) {
		return usecase.WeatherReport{}, errors.New("hourly series of different lengths")
	}
	for _, layer := range layers {
		if len(layer) != len(times) {
			return usecase.WeatherReport{}, errors.New("hourly series of different lengths")
		}
	}
	if len(a.Daily.PrecipitationSum) != len(a.Daily.Time) {
		return usecase.WeatherReport{}, errors.New("daily series of different lengths")
	}

	report := usecase.WeatherReport{
		Hours: make([]domain.HourlyWeather, 0, len(times)),
		Days:  make([]domain.DailyRain, 0, len(a.Daily.Time)),
	}
	for i, unix := range times {
		hour := domain.HourlyWeather{Time: time.Unix(unix, 0).In(loc), RainMm: nonNegative(rain[i])}
		hour.SoilMoistureM3M3 = weightedMoisture(layers, i)
		report.Hours = append(report.Hours, hour)
	}
	for i, unix := range a.Daily.Time {
		// A day without a value is left out rather than taken as dry.
		if mm := nonNegative(a.Daily.PrecipitationSum[i]); mm != nil {
			report.Days = append(report.Days, domain.DailyRain{Date: domain.LocalDay(time.Unix(unix, 0), loc), Mm: *mm})
		}
	}
	return report, nil
}

// decodeSeries reads one hourly array; a missing one is an error, because the
// request asked for it.
func decodeSeries(hourly map[string]json.RawMessage, name string, target any) error {
	raw, ok := hourly[name]
	if !ok {
		return fmt.Errorf("answer without hourly %s", name)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("malformed hourly %s", name)
	}
	return nil
}

// weightedMoisture is the moisture of 0 to 27 cm at hour i, or nil when any
// layer is missing (a partial mean would lean towards the layers present).
func weightedMoisture(layers [][]*float64, i int) *float64 {
	var sum, thickness float64
	for l, layer := range soilLayers {
		value := layers[l][i]
		if value == nil || *value < 0 {
			return nil
		}
		sum += *value * layer.thicknessCm
		thickness += layer.thicknessCm
	}
	mean := sum / thickness
	return &mean
}

// nonNegative drops impossible negative values, which the model never sends
// for rain; nil stays nil.
func nonNegative(value *float64) *float64 {
	if value == nil || *value < 0 {
		return nil
	}
	copied := *value
	return &copied
}

// unwrapURLError drops the *url.Error wrapper, which embeds the request URL
// (and so the coordinate).
func unwrapURLError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

var _ usecase.WeatherProvider = (*Client)(nil)
