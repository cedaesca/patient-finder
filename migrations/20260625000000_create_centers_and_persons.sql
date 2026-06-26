-- +goose Up
-- +goose StatementBegin
CREATE TABLE centers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('health', 'shelter')),
    estado_id UUID NOT NULL REFERENCES estados(id),
    municipio_id UUID NOT NULL REFERENCES municipios(id),
    parroquia_id UUID NOT NULL REFERENCES parroquias(id),
    address TEXT,
    contacts JSONB,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE persons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name TEXT,
    last_name TEXT,
    cedula VARCHAR(8),
    sex TEXT CHECK (sex IN ('M', 'F')),
    age_approx INT,
    status TEXT NOT NULL DEFAULT 'hospitalized'
        CHECK (status IN ('hospitalized', 'discharged', 'deceased', 'transferred')),
    rescue_estado_id UUID NOT NULL REFERENCES estados(id),
    rescue_municipio_id UUID NOT NULL REFERENCES municipios(id),
    rescue_parroquia_id UUID REFERENCES parroquias(id),
    center_id UUID NOT NULL REFERENCES centers(id),
    contacts JSONB,
    notes TEXT NOT NULL DEFAULT '',
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT at_least_one_identifier
        CHECK (cedula IS NOT NULL OR (first_name IS NOT NULL AND last_name IS NOT NULL))
);

CREATE INDEX idx_persons_cedula ON persons(cedula) WHERE cedula IS NOT NULL;
CREATE INDEX idx_persons_center ON persons(center_id);
CREATE INDEX idx_persons_status ON persons(status);
CREATE INDEX idx_persons_created_by ON persons(created_by);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS persons;
DROP TABLE IF EXISTS centers;
-- +goose StatementEnd
