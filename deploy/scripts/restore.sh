#!/usr/bin/env bash
# Rotina de Restauração Segura de Banco de Dados na VPS com Validação Prévia
set -euo pipefail

BACKUP_FILE="${1:-}"
if [ -z "$BACKUP_FILE" ] || [ ! -f "$BACKUP_FILE" ]; then
    echo "Uso: $0 <caminho_do_arquivo.sql.gz> [nome_do_banco_destino]" >&2
    exit 1
fi

TARGET_DB="${2:-rj_saas_prod}"
DB_USER="${DB_USER:-rj_app}"

# 1. Validação de integridade do arquivo
if [ -f "${BACKUP_FILE}.sha256" ]; then
    echo "Validando checksum SHA-256 do arquivo..."
    sha256sum -c "${BACKUP_FILE}.sha256"
fi

# 2. Teste prévio em banco temporário descartável
STAGING_DB="rj_staging_restore_test"
echo "Executando ensaio prévio em banco temporário ($STAGING_DB)..."
createdb -U "$DB_USER" "$STAGING_DB" || true
gunzip -c "$BACKUP_FILE" | psql -U "$DB_USER" -d "$STAGING_DB" --quiet -v ON_ERROR_STOP=1

echo "Ensaio em staging concluído com sucesso. Descartando banco temporário..."
dropdb -U "$DB_USER" "$STAGING_DB"

# 3. Restauração no banco oficial de destino
echo "AVISO: Restaurando backup no banco de destino: ${TARGET_DB}..."
gunzip -c "$BACKUP_FILE" | psql -U "$DB_USER" -d "$TARGET_DB" -v ON_ERROR_STOP=1

echo "Restauração concluída com sucesso em ${TARGET_DB}."
