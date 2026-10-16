#!/bin/sh
set -eu
: "${APP_DB_USER:?APP_DB_USER must be set}"
: "${APP_DB_PASSWORD:?APP_DB_PASSWORD must be set}"
: "${TEST_DB_NAME:?TEST_DB_NAME must be set}"

# This script runs only when the PostgreSQL data directory is initialized for the first time.
# SQL identifiers and passwords are escaped by PostgreSQL's format() rather than interpolated
# into SQL text by the shell.
psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  --set=ON_ERROR_STOP=1 \
  --set=app_user="$APP_DB_USER" \
  --set=app_password="$APP_DB_PASSWORD" \
  --set=test_db="$TEST_DB_NAME" <<'SQL'
SELECT format(
  'CREATE ROLE %I WITH LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT',
  :'app_user', :'app_password'
) \gexec
SELECT format('GRANT CONNECT, TEMPORARY ON DATABASE %I TO %I', current_database(), :'app_user') \gexec
SELECT format('GRANT USAGE, CREATE ON SCHEMA public TO %I', :'app_user') \gexec
SELECT format('CREATE DATABASE %I', :'test_db') \gexec
SQL

# Keep the integration test database separate from application data, while using the same
# non-superuser role and the same PostgreSQL service/container.
psql --username "$POSTGRES_USER" --dbname "$TEST_DB_NAME" \
  --set=ON_ERROR_STOP=1 --set=app_user="$APP_DB_USER" <<'SQL'
SELECT format('GRANT CONNECT, TEMPORARY ON DATABASE %I TO %I', current_database(), :'app_user') \gexec
SELECT format('GRANT USAGE, CREATE ON SCHEMA public TO %I', :'app_user') \gexec
SQL
