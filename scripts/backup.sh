#!/usr/bin/env bash
# Realiza backup lógico do banco de dados PostgreSQL do ambiente de desenvolvimento via Docker
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-backups}"
mkdir -p "$BACKUP_DIR"

TIMESTAMP=$(date -u +"%Y%m%d_%H%M%SZ")
BACKUP_FILE="${BACKUP_DIR}/rj_saas_dev_${TIMESTAMP}.sql.gz"

echo "Iniciando backup lógico de rj_saas_dev..."
docker compose exec -T db pg_dump -U rj_bootstrap -d rj_saas_dev --clean --if-exists --no-owner --no-privileges | gzip > "$BACKUP_FILE"

# Gera checksum SHA-256 para integridade
sha256sum "$BACKUP_FILE" > "${BACKUP_FILE}.sha256"

echo "Backup concluído com sucesso: $BACKUP_FILE"
echo "Checksum: $(cat "${BACKUP_FILE}.sha256")"
