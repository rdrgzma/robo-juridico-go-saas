# Build e implantação

Desenvolvimento oficial: Docker Compose, conforme DEVELOPMENT.md. Runtime multi-stage
em docker/production/Dockerfile: binários server/migrate/healthcheck com CGO=0,
-trimpath e -ldflags=-s -w, Debian slim com CA/tzdata e usuário 65532:65532.
Imagem construída e UID conferido localmente. Nenhum segredo ou fonte no runtime.

O servidor continua recusando production até homologação operacional e jurídica, mesmo com identidade implementada.
Não houve deploy público nem alteração de VPS/domínio/Nginx/systemd/firewall antigos.

scripts/build.ps1 e build.sh compilam binário Linux pelo contêiner app; dev.ps1 build
ou dev.sh build constrói a imagem. Não usar executável Windows dentro da imagem Linux.

Sprint 6 entregou a infraestrutura completa de implantação em `deploy/`:
- Nginx com terminação TLS, rate limiting por IP, proxy reverso para `127.0.0.1:8080` e cabeçalhos de segurança (HSTS, CSP, X-Frame-Options, X-Content-Type-Options) em `deploy/nginx/robo-juridico.conf`.
- Unidade systemd dedicada com isolamento `robo-juridico.service` e migrações prévias automáticas em `robo-juridico-migrate.service`.
- Firewall UFW (`deploy/firewall/ufw-setup.sh`) com política default deny e liberação exclusiva das portas públicas 22, 80 e 443.
- Alternativa conteinerizada `deploy/compose.prod.yaml` com imagem multi-stage Debian slim e usuário não-privilegiado.
- Scripts operacionais testados em `deploy/scripts/`: `install.sh`, `update.sh` (com rollback automático se o health check falhar), `backup.sh` (com gzip, SHA-256 e rotação de 7 dias) e `restore.sh` (com ensaio prévio em banco temporário).
- Detalhes e checklist de lançamento em `docs/OPERATIONS.md`.
