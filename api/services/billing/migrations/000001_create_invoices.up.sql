CREATE TABLE invoices (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    number BIGINT GENERATED ALWAYS AS IDENTITY NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'OPEN',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ,
    CONSTRAINT invoices_status_check CHECK (status IN ('OPEN', 'CLOSED')),
    CONSTRAINT invoices_closed_at_status_check CHECK (closed_at IS NULL OR status = 'CLOSED')
);

CREATE TABLE invoice_items (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    invoice_id BIGINT NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    product_id BIGINT NOT NULL,
    product_code_snapshot TEXT NOT NULL,
    product_description_snapshot TEXT NOT NULL,
    quantity INTEGER NOT NULL,
    CONSTRAINT invoice_items_product_id_check CHECK (product_id > 0),
    CONSTRAINT invoice_items_product_code_snapshot_check CHECK (btrim(product_code_snapshot) <> ''),
    CONSTRAINT invoice_items_product_description_snapshot_check CHECK (btrim(product_description_snapshot) <> ''),
    CONSTRAINT invoice_items_quantity_check CHECK (quantity > 0),
    CONSTRAINT invoice_items_invoice_product_key UNIQUE (invoice_id, product_id)
);

CREATE INDEX invoice_items_invoice_id_idx ON invoice_items (invoice_id, id);
