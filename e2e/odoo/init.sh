#!/usr/bin/env bash
# Prepares the e2e Odoo for the gateway's billing and writes e2e/.odoo/env.
# Run from the repo root after `docker compose -f e2e/docker-compose.yml up
# -d --wait`. Idempotent: the database is created once (a few minutes), the
# setup (e2e/odoo/setup.py) runs every time and issues a new API key.
set -euo pipefail

COMPOSE="docker compose -f e2e/docker-compose.yml"
DB=taronja
PORT="${E2E_ODOO_PORT:-18169}"
DB_ARGS="--db_host odoo-db --db_user odoo --db_password odoo"

exists=$($COMPOSE exec -T odoo-db psql -U odoo -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='$DB'")
if [ "$exists" != "1" ]; then
    echo "odoo: creating database $DB (account, l10n_es)..."
    $COMPOSE exec -T odoo odoo -d "$DB" $DB_ARGS -i account,l10n_es --without-demo=all --stop-after-init --no-http --log-level=warn
    $COMPOSE restart odoo >/dev/null
fi

echo "odoo: configuring company, journals, tax and API user..."
out=$($COMPOSE exec -T odoo odoo shell -d "$DB" $DB_ARGS --no-http --log-level=warn < e2e/odoo/setup.py)
if ! grep -q '^TG_ENV ODOO_API_KEY=' <<<"$out"; then
    echo "$out" >&2
    echo "odoo: setup failed" >&2
    exit 1
fi

mkdir -p e2e/.odoo
{
    echo "ODOO_URL=http://localhost:$PORT"
    echo "ODOO_DB=$DB"
    grep '^TG_ENV ' <<<"$out" | sed 's/^TG_ENV //'
} > e2e/.odoo/env

# The running server caches settings (languages) the setup may have changed.
$COMPOSE restart odoo >/dev/null

for _ in $(seq 1 60); do
    if curl -fsS "http://localhost:$PORT/web/health" >/dev/null 2>&1; then
        echo "odoo: ready, settings in e2e/.odoo/env"
        exit 0
    fi
    sleep 2
done
echo "odoo: not healthy on port $PORT" >&2
exit 1
