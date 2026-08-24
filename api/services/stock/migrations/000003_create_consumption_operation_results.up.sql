CREATE TABLE consumption_operation_results (
    operation_id UUID NOT NULL,
    product_id BIGINT NOT NULL,
    balance INTEGER NOT NULL,
    CONSTRAINT consumption_operation_results_pkey PRIMARY KEY (operation_id, product_id),
    CONSTRAINT consumption_operation_results_operation_fkey
        FOREIGN KEY (operation_id)
        REFERENCES consumption_operations (operation_id)
        ON DELETE CASCADE,
    CONSTRAINT consumption_operation_results_product_fkey
        FOREIGN KEY (product_id)
        REFERENCES products (id)
        ON DELETE RESTRICT,
    CONSTRAINT consumption_operation_results_balance_check CHECK (balance >= 0)
);
