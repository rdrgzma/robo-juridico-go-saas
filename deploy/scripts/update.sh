#!/usr/bin/env bash
# Script de Atualização Atômica de Release com Verificação e Rollback Automático
set -euo pipefail

if [ "$EUID" -ne 0 ]; then
    echo "Erro: execute como root (sudo)." >&2
    exit 1
fi

APP_DIR="/opt/robo-juridico"
BIN_DIR="$APP_DIR/bin"
NEW_SERVER="${1:-/tmp/rj-server}"
NEW_MIGRATE="${2:-/tmp/rj-migrate}"

if [ ! -f "$NEW_SERVER" ] || [ ! -f "$NEW_MIGRATE" ]; then
    echo "Uso: $0 <novo_binario_server> <novo_binario_migrate>" >&2
    exit 1
fi

echo "1. Realizando backup preventivo do banco antes da atualização..."
"$APP_DIR/scripts/backup.sh" || echo "Aviso: backup preventivo encontrou pendências; avalie antes de prosseguir."

echo "2. Preservando versão anterior dos binários..."
if [ -f "$BIN_DIR/server" ]; then
    cp "$BIN_DIR/server" "$BIN_DIR/server.prev"
fi
if [ -f "$BIN_DIR/migrate" ]; then
    cp "$BIN_DIR/migrate" "$BIN_DIR/migrate.prev"
fi

echo "3. Instalando novos binários com permissões estritas..."
cp "$NEW_SERVER" "$BIN_DIR/server"
cp "$NEW_MIGRATE" "$BIN_DIR/migrate"
chmod 755 "$BIN_DIR/server" "$BIN_DIR/migrate"
chown -R robo-juridico:robo-juridico "$BIN_DIR"

echo "4. Executando migrações de banco pendentes..."
systemctl start robo-juridico-migrate.service

echo "5. Reiniciando serviço principal..."
systemctl restart robo-juridico.service

echo "6. Verificando health check (smoke)..."
sleep 2
if curl -sf http://127.0.0.1:8080/healthz >/dev/null; then
    echo "Atualização concluída com sucesso. Serviço saudável."
    rm -f "$BIN_DIR/server.prev" "$BIN_DIR/migrate.prev"
else
    echo "FALHA no health check após atualização! Iniciando rollback imediato..." >&2
    if [ -f "$BIN_DIR/server.prev" ]; then
        mv "$BIN_DIR/server.prev" "$BIN_DIR/server"
    fi
    if [ -f "$BIN_DIR/migrate.prev" ]; then
        mv "$BIN_DIR/migrate.prev" "$BIN_DIR/migrate"
    fi
    systemctl restart robo-juridico.service
    echo "Rollback concluído. Verifique os logs com journalctl -u robo-juridico.service" >&2
    exit 1
fi
