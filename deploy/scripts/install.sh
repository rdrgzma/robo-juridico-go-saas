#!/usr/bin/env bash
# Script de Instalação e Provisionamento do Robô Jurídico SaaS na VPS (Ubuntu LTS)
set -euo pipefail

if [ "$EUID" -ne 0 ]; then
    echo "Erro: execute como root (sudo)." >&2
    exit 1
fi

APP_DIR="/opt/robo-juridico"
APP_USER="robo-juridico"

echo "1. Criando usuário de sistema não privilegiado..."
if ! id "$APP_USER" &>/dev/null; then
    useradd -r -s /usr/sbin/nologin -d "$APP_DIR" -m "$APP_USER"
fi

echo "2. Criando árvore de diretórios em $APP_DIR..."
mkdir -p "$APP_DIR/bin" "$APP_DIR/etc" "$APP_DIR/backups" "$APP_DIR/logs"

echo "3. Instalando serviços systemd..."
cp deploy/systemd/robo-juridico.service /etc/systemd/system/
cp deploy/systemd/robo-juridico-migrate.service /etc/systemd/system/
systemctl daemon-reload

echo "4. Instalando configuração do Nginx..."
cp deploy/nginx/robo-juridico.conf /etc/nginx/sites-available/
ln -sf /etc/nginx/sites-available/robo-juridico.conf /etc/nginx/sites-enabled/
nginx -t

echo "5. Ajustando permissões estritas..."
chown -R "$APP_USER:$APP_USER" "$APP_DIR"
chmod 750 "$APP_DIR"
chmod 700 "$APP_DIR/etc" "$APP_DIR/backups"

echo "Provisionamento concluído. Configure o arquivo /opt/robo-juridico/etc/robo-juridico.env antes de iniciar os serviços."
