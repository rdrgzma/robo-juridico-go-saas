#!/usr/bin/env bash
# Restaura um backup lógico no banco de dados PostgreSQL via Docker
set -euo pipefail

BACKUP_FILE="${1:-}"
if [ -z "$BACKUP_FILE" ] || [ ! -f "$BACKUP_FILE" ]; then
    echo "Uso: $0 <caminho_do_arquivo.sql.gz>" >&2
    exit 1
fi

TARGET_DB="${2:-rj_saas_dev}"

# Valida checksum se o arquivo .sha256 existir
if [ -f "${BACKUP_FILE}.sha256" ]; then
    echo "Verificando integridade SHA-256..."
    sha256sum -c "${BACKUP_FILE}.sha256"
fi

echo "Restaurando backup em ${TARGET_DB}..."
gunzip -c "$BACKUP_FILE" | docker compose exec -T db psql -U rj_bootstrap -d "$TARGET_DB"

echo "Restauração concluída com sucesso."
