#!/bin/sh
set -eu

# This script is run by PostgreSQL's docker-entrypoint-initdb.d
# It creates the databases and users with least-privilege access

echo "Creating databases and users..."

# Read passwords from environment or use defaults
STOCK_DB_PASSWORD="${STOCK_DB_PASSWORD:-stock_pass}"
BILLING_DB_PASSWORD="${BILLING_DB_PASSWORD:-billing_pass}"

# Create users
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  --set=stock_password="$STOCK_DB_PASSWORD" \
  --set=billing_password="$BILLING_DB_PASSWORD" <<'EOSQL'
CREATE USER stock_user WITH LOGIN PASSWORD :'stock_password';
CREATE USER billing_user WITH LOGIN PASSWORD :'billing_password';
EOSQL

# Create databases
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<EOSQL
CREATE DATABASE stock_db OWNER stock_user;
CREATE DATABASE billing_db OWNER billing_user;
REVOKE CONNECT ON DATABASE stock_db FROM PUBLIC;
REVOKE CONNECT ON DATABASE billing_db FROM PUBLIC;
GRANT CONNECT ON DATABASE stock_db TO stock_user;
GRANT CONNECT ON DATABASE billing_db TO billing_user;
EOSQL

# Grant privileges on stock_db
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "stock_db" <<EOSQL
GRANT ALL PRIVILEGES ON DATABASE stock_db TO stock_user;
GRANT USAGE ON SCHEMA public TO stock_user;
GRANT ALL ON SCHEMA public TO stock_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO stock_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO stock_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON FUNCTIONS TO stock_user;
EOSQL

# Grant privileges on billing_db
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "billing_db" <<EOSQL
GRANT ALL PRIVILEGES ON DATABASE billing_db TO billing_user;
GRANT USAGE ON SCHEMA public TO billing_user;
GRANT ALL ON SCHEMA public TO billing_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO billing_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO billing_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON FUNCTIONS TO billing_user;
EOSQL

echo "Database initialization complete!"
