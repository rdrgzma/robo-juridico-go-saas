# Build e implantação

Desenvolvimento oficial: Docker Compose, conforme DEVELOPMENT.md. Runtime multi-stage
em docker/production/Dockerfile: binários server/migrate/healthcheck com CGO=0,
-trimpath e -ldflags=-s -w, Debian slim com CA/tzdata e usuário 65532:65532.
Imagem construída e UID conferido localmente. Nenhum segredo ou fonte no runtime.

O servidor continua recusando production até autenticação e isolamento homologados.
Não houve deploy público nem alteração de VPS/domínio/Nginx/systemd/firewall antigos.

scripts/build.ps1 e build.sh compilam binário Linux pelo contêiner app; dev.ps1 build
ou dev.sh build constrói a imagem. Não usar executável Windows dentro da imagem Linux.

Sprint 6 preparará Ubuntu LTS, VPS 4 vCPU/8 GB/80 GB+, PostgreSQL privado exclusivo,
Nginx HTTPS, serviço systemd dedicado, releases/rollback, firewall e backup externo
criptografado com restore ensaiado. Docker de produção será alternativa documentada.
Não há ainda arquivos Nginx/systemd/install/update/backup/restore completos.
