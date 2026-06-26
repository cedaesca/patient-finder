-- +goose Up
-- +goose StatementBegin
WITH geo AS (
    SELECT e.id AS estado_id, m.id AS municipio_id, p.id AS parroquia_id,
           e.name AS estado, m.name AS municipio, p.name AS parroquia
    FROM estados e
    JOIN municipios m ON m.estado_id = e.id
    JOIN parroquias p ON p.municipio_id = m.id
)
INSERT INTO centers (name, type, estado_id, municipio_id, parroquia_id, address, is_active)
SELECT c.name, c.type, g.estado_id, g.municipio_id, g.parroquia_id, c.address, true
FROM (VALUES
    ('Clínica El Ávila',                                    'health', 'Distrito Capital', 'Libertador', 'San Bernardino', 'San Bernardino, Caracas'),
    ('Cruz Roja',                                           'health', 'Distrito Capital', 'Libertador', 'La Candelaria', 'Av. Andrés Bello, Caracas'),
    ('Hospital Ana Francisca Pérez de León II',             'health', 'Miranda',          'Sucre',      'Petare',         'Petare, Caracas'),
    ('Hospital Domingo Luciani',                            'health', 'Miranda',          'Sucre',      'Petare',         'El Llanito, Caracas'),
    ('Hospital General del Oeste',                          'health', 'Distrito Capital', 'Libertador', 'Sucre (Catia)',  'Catia, Caracas'),
    ('Hospital J.M. de los Ríos',                           'health', 'Distrito Capital', 'Libertador', 'San Bernardino', 'San Bernardino, Caracas'),
    ('Hospital José María Vargas',                          'health', 'La Guaira',        'Vargas',     'La Guaira',      'La Guaira'),
    ('Hospital Militar Universitario Dr. Carlos Arvelo',    'health', 'Distrito Capital', 'Libertador', 'San Bernardino', 'San Bernardino, Caracas'),
    ('Hospital Pérez Carreño',                              'health', 'Distrito Capital', 'Libertador', 'La Vega',        'La Vega, Caracas'),
    ('Hospital Universitario de Caracas',                   'health', 'Distrito Capital', 'Libertador', 'San Pedro',      'Los Chaguaramos, Caracas'),
    ('Hospital Vargas de Caracas',                          'health', 'Distrito Capital', 'Libertador', 'Catedral',       'Centro de Caracas'),
    ('Periférico de Catia',                                 'health', 'Distrito Capital', 'Libertador', 'Sucre (Catia)',  'Catia, Caracas'),
    ('Seguro Social La Guaira',                             'health', 'La Guaira',        'Vargas',     'La Guaira',      'La Guaira')
) AS c(name, type, estado, municipio, parroquia, address)
JOIN geo g ON g.estado = c.estado AND g.municipio = c.municipio AND g.parroquia = c.parroquia;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM centers WHERE name IN (
    'Clínica El Ávila',
    'Cruz Roja',
    'Hospital Ana Francisca Pérez de León II',
    'Hospital Domingo Luciani',
    'Hospital General del Oeste',
    'Hospital J.M. de los Ríos',
    'Hospital José María Vargas',
    'Hospital Militar Universitario Dr. Carlos Arvelo',
    'Hospital Pérez Carreño',
    'Hospital Universitario de Caracas',
    'Hospital Vargas de Caracas',
    'Periférico de Catia',
    'Seguro Social La Guaira'
);
-- +goose StatementEnd
