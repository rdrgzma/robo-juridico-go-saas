# Estado real do desenvolvimento

Atualizado em 17/10/2026. Sprint 1 de fundação/consolidação e ambiente Docker entregue.
O produto SaaS completo ainda não está concluído nem autorizado para produção.

## Entregue

- Git independente em main, módulo Go e recursos próprios; sem acesso ao legado em runtime.
- Auditoria fixada em 12685bbe7a5be048fcaf01332f67503a09b0bee8 e matriz de migração.
- Titular autorizou expressamente o reaproveitamento; NOTICE.md registra fontes e adaptações.
- Docker Compose oficial: app com Air e PostgreSQL 17.11, módulos/build/dados em volumes.
- Porta host 127.0.0.1:8081 escolhida pelo usuário para preservar o painel legado em 8080.
- Migrations SQL executáveis via cmd/migrate up/status, checksums/lock/transações.
- Roles e bancos separados para desenvolvimento/testes; teste recusa banco principal.
- /docs, /openapi.json, /health/live e /health/ready funcionais; readiness inclui banco/schema.
- Motores e contratos de cálculo centralizados em Go, importação/exportação CSV/XLSX,
  tabelas financeiras decimais únicas para PRICE/SAC/GAUSS, CNJ e calendário explícito.
- Correções das falhas encontradas e testes de regressão exatos/negativos/caracterização.
- Dockerfile multi-stage com UID/GID 65532 e binários Linux independentes de executáveis Windows.
- Scripts dev/build e CI operam por Docker; documentação de desenvolvimento e testes.

## Verificado nesta sessão

Compose inicializado com app/db saudáveis, migrations aplicadas/reaplicadas,
testes unitários e PostgreSQL separados, vet, build e race detector Linux passaram.
Hot reload de Go/HTML/CSS foi exercitado por cmd/devverify e os arquivos foram restaurados.
Registro sintético de persistência permaneceu após reinicializar o projeto novo.
Página e /docs foram abertas e conferidas no navegador. Não foi necessário Go ou
PostgreSQL no Windows para essas operações.

Govulncheck encontrou duas vulnerabilidades em x/text v0.29.0, dependência indireta
trazida pelo pgx; atualização para v0.41.0 aplicada. Nova varredura retornou
No vulnerabilities found. A imagem de desenvolvimento também compila Air com esse
patch de dependência. Os resultados não equivalem a pentest nem certificação jurídica.

## Limites explícitos e próxima sprint

- /app, /admin e /portal retornam 503; login, sessões/MFA, usuários, RBAC/tenancy e
  isolamento serão Sprint 2 e painéis nas Sprints 3 a 5. Organizations é só fundação SQL.
- DataJud/DJEN, séries oficiais curadas, calendário por tribunal, histórico e laudos
  serão integrados nas próximas etapas. Não estão simulados como operações reais.
- O núcleo reaproveitado possui regressões sintéticas e de caracterização. Taxa legal,
  SELIC e aplicação cronológica de pagamentos intermediários não estão homologadas;
  a API rejeita esses casos. O seed do legado só existe nos testes.
- Contratos de cálculo foram reaproveitados com mudanças de segurança documentadas;
  não há ainda paridade de todos os endpoints CNJ/processos/OAB/importação/webhooks.
- Nenhum banco legado foi importado. Ferramenta de migração SQLite ainda é futura.
- GitHub sem autenticação local: remoto e CI remoto não foram publicados/executados.
- Produção bloqueada e nenhum deploy/VPS/serviço legado foi alterado.

Os 14 critérios finais do produto continuam sendo critérios das seis sprints, não
uma promessa de que todos estejam prontos na Sprint 1. Próximo trabalho: autenticação
e identidade multiempresa, migrations de usuários/roles e testes negativos de isolamento.

## Arquivos

Infraestrutura nova: compose.yaml, .air.toml, .dockerignore, docker/dev/Dockerfile,
docker/dev/init-db.sh, docker/production/Dockerfile, scripts/dev.ps1/dev.sh,
cmd/devinit, cmd/devverify, cmd/healthcheck, cmd/migrate, internal/storage,
migrations/000001_foundation.sql e migrations/embed.go.

Núcleo: internal/calculator inclui contratos/engine/juros/correção/encargos,
financing wrapper e Finance único, importer/exporter/XLSX, validação e testes.
seed_legacy_test.go nunca é compilado no servidor. CNJ/prazos/indices mantêm seus testes.

Web/transporte: health/readiness, rotas convencionais reaproveitadas, limites decimais,
web/templates/public/docs.html, OpenAPI e página de estado atualizados.
Documentação: README, NOTICE, DEVELOPMENT, auditoria, arquitetura, banco, segurança,
API, migração, deploy, roadmap e TESTING. Inventário exato disponível em git ls-files.

Commits locais usam autor técnico Codex (local automation) <codex@localhost>, por
comando, pois não há identidade humana configurada. Configuração Git global preservada.
