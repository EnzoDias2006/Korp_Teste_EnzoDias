# Stock Service Migrations

This directory contains database migrations for the Stock service.

Migrations are managed using `golang-migrate` and are applied to the `stock_db` database.

## Usage

```bash
# Apply all migrations
golang-migrate -path migrations -database "$DATABASE_URL" up

# Rollback all migrations
golang-migrate -path migrations -database "$DATABASE_URL" down
```

## Migration Files

Migration files follow the naming convention:
- `{NNNNNN}_{description}.up.sql` - Apply migration
- `{NNNNNN}_{description}.down.sql` - Rollback migration

Where `NNNNNN` is a sequential number with leading zeros.
