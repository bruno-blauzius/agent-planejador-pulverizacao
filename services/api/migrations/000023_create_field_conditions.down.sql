DROP TABLE IF EXISTS trafficability_thresholds;
DROP TABLE IF EXISTS soil_inspections;
DROP TABLE IF EXISTS rain_readings;
DROP TABLE IF EXISTS rain_gauge_fields;
DROP TABLE IF EXISTS rain_gauges;
ALTER TABLE fields DROP CONSTRAINT IF EXISTS ck_fields_soil_texture;
ALTER TABLE fields DROP COLUMN IF EXISTS soil_texture;
