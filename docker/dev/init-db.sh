#!/usr/bin/env bash
set -euo pipefail
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
  --set=app_password="$APP_DB_PASSWORD" --set=test_password="$TEST_DB_PASSWORD" <<'SQL'
CREATE ROLE rj_app LOGIN PASSWORD :'app_password' NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE ROLE rj_test LOGIN PASSWORD :'test_password' NOSUPERUSER NOCREATEDB NOCREATEROLE;
CREATE DATABASE rj_saas_dev OWNER rj_app;
CREATE DATABASE rj_saas_test OWNER rj_test;
REVOKE ALL ON DATABASE rj_saas_dev FROM PUBLIC;
REVOKE ALL ON DATABASE rj_saas_test FROM PUBLIC;
GRANT CONNECT ON DATABASE rj_saas_dev TO rj_app;
GRANT CONNECT ON DATABASE rj_saas_test TO rj_test;
SQL
