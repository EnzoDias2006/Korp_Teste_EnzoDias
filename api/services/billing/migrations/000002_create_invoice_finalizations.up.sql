CREATE TABLE invoice_finalizations (
    invoice_id BIGINT PRIMARY KEY REFERENCES invoices (id) ON DELETE CASCADE,
    operation_id UUID NOT NULL UNIQUE,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

