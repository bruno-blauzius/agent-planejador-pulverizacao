-- Falha (FK) se algum talhão já usa a cultura, o que é o comportamento desejado.
DELETE FROM crops
WHERE code IN ('coffee_arabica', 'coffee_conilon', 'soybean', 'corn', 'corn_second_season', 'wheat');
