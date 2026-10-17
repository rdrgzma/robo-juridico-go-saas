#!/usr/bin/env bash
# Rotina de Backup Lógico PostgreSQL na VPS com Checksum e Rotação de Retenção
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/opt/robo-juridico/backups}"
mkdir -p "$BACKUP_DIR"

DB_NAME="${DB_NAME:-rj_saas_prod}"
DB_USER="${DB_USER:-rj_app}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

TIMESTAMP=$(date -u +"%Y%m%d_%H%M%SZ")
BACKUP_FILE="${BACKUP_DIR}/${DB_NAME}_${TIMESTAMP}.sql.gz"

echo "Executando pg_dump para ${DB_NAME}..."
pg_dump -U "$DB_USER" -d "$DB_NAME" --clean --if-exists --no-owner --no-privileges | gzip > "$BACKUP_FILE"

# Geração de checksum SHA-256
sha256sum "$BACKUP_FILE" > "${BACKUP_FILE}.sha256"

# Permissões estritas
chmod 600 "$BACKUP_FILE" "${BACKUP_FILE}.sha256"

echo "Backup concluído: $BACKUP_FILE"
echo "Checksum: $(cat "${BACKUP_FILE}.sha256")"

# Rotação de backups anteriores
echo "Aplicando política de retenção de ${RETENTION_DAYS} dias..."
find "$BACKUP_DIR" -name "${DB_NAME}_*.sql.gz*" -mtime +"$RETENTION_DAYS" -delete
echo "Rotação finalizada."
