-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TYPE email_verification_purpose AS ENUM ('register', 'password_reset', 'password_change');
CREATE TYPE email_verification_status AS ENUM ('pending', 'used', 'expired', 'revoked');

CREATE TABLE IF NOT EXISTS email_otp_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email VARCHAR(255) NOT NULL,
  otp_hash VARCHAR(255) NOT NULL,
  purpose email_verification_purpose NOT NULL,
  status email_verification_status NOT NULL,
  expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
  created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_email_otp_requests_email_purpose_status
  ON email_otp_requests (email, purpose, status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_email_otp_requests_email_purpose_status;
DROP TABLE IF EXISTS email_otp_requests;
DROP TYPE IF EXISTS email_verification_purpose;
DROP TYPE IF EXISTS email_verification_status;
-- +goose StatementEnd
