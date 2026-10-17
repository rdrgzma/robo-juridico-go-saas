#!/usr/bin/env bash
# Configuração do Firewall UFW para o servidor de produção Robô Jurídico SaaS
set -euo pipefail

if [ "$EUID" -ne 0 ]; then
    echo "Erro: este script deve ser executado como root (sudo)." >&2
    exit 1
fi

echo "Configurando políticas padrão do firewall..."
ufw default deny incoming
ufw default allow outgoing

echo "Liberando portas públicas autorizadas..."
# SSH administrativo (ajuste caso utilize porta não padrão)
ufw allow 22/tcp comment 'SSH'

# Tráfego web via Nginx
ufw allow 80/tcp comment 'HTTP Nginx'
ufw allow 443/tcp comment 'HTTPS Nginx'

echo "Garantindo bloqueio de serviços internos contra acesso externo direto..."
# Assegura que portas internas nunca aceitem tráfego público direto
ufw deny 5432/tcp comment 'PostgreSQL Interno'
ufw deny 8080/tcp comment 'Aplicação Go Interna'
ufw deny 8081/tcp comment 'Dev Port'

echo "Habilitando UFW..."
ufw --force enable

echo "Status do firewall:"
ufw status verbose
