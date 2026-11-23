-- Mesmas culturas do seed do front (web/app/services/mock/seed.ts). Idempotente: reaplicar não duplica.
INSERT INTO crops (code, name) VALUES
    ('coffee_arabica', 'Café arábica'),
    ('coffee_conilon', 'Café conilon'),
    ('soybean', 'Soja'),
    ('corn', 'Milho'),
    ('corn_second_season', 'Milho safrinha'),
    ('wheat', 'Trigo')
ON CONFLICT (code) DO NOTHING;
