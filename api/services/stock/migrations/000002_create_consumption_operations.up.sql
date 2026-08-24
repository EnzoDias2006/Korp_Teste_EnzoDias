CREATE TABLE consumption_operations (
    operation_id UUID PRIMARY KEY,
    invoice_id BIGINT NOT NULL,
    fingerprint BYTEA NOT NULL CHECK (octet_length(fingerprint) = 32),
    external_reference TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT consumption_operations_invoice_fingerprint_key UNIQUE (invoice_id, fingerprint),
    CONSTRAINT consumption_operations_updated_after_created_check CHECK (updated_at >= created_at)
);

