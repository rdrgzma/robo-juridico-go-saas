# Estado real do desenvolvimento

Atualizado em 17/10/2026. Sprints 1, 2, 3 e 4 entregues. Plataforma completa e
produção ainda pendentes. Repositório independente com origin em
https://github.com/rdrgzma/robo-juridico-go-saas.git; legado preservado.

Sprint 1: auditoria e reaproveitamento autorizado dos motores, Docker Compose,
PostgreSQL separado, migrations, regressões jurídicas, health checks e hot reload.

Sprint 2: usuários/memberships, RBAC, sessões, Argon2id, CSRF/Origin, recuperação
local criptografada, MFA e códigos de recuperação (dispensados por padrão no ambiente
local via `AUTH_DISABLE_MFA=true`, reversível com `AUTH_DISABLE_MFA=false` sem apagar
segredos nem forjar `mfa_verified`), gestão de membros/chaves, auditoria,
cotas transacionais e reconciliação. Migration 000002 aplicada. Bootstrap local gerado
sem senha fixa; instruções em AUTH.md.

Vet, testes completos com race detector, integração PostgreSQL, build Linux,
scanner de dependências e smoke/hot reload passaram dentro do Docker. Scanner sem
símbolos vulneráveis alcançáveis; advisory de módulo OpenPGP não importado descrito
em TESTING.md. Imagem runtime construída. Login conferido no navegador; fluxos de
MFA e formulários exercitados em HTTPS nos testes. Resultado local não certifica CI
remoto, pentest nem homologação jurídica.

Sprint 3: /app tem dashboard, consultas CNJ/fontes opcionais, liquidação/importação,
revisão bancária, prazos, IPCA BCB/IBGE, histórico PostgreSQL e recálculo, CSV/XLSX,
JSON com HMAC e memória HTML privada para impressão. Migration 000003 aplicada.
Testes negativos cobrem histórico/downloads/recálculo entre organizações, XSS,
idempotência e cotas concorrentes. Fontes reais BCB/IBGE verificadas no contêiner;
DataJud/DJEN com configuração estão implementados e testados por contratos sintéticos,
sem homologação ao vivo nesta instalação. Guia e limites em WORKSPACE.md.

Sprint 4: /admin entregue com dashboard operacional real, gestão de clientes e
organizações (razão social, documento, responsável, ativação/suspensão com revogação
imediata de sessões, contratos com valor decimal, vigência e histórico), planos e
autorização de recursos aplicados no backend (`/app` e `/api/v1`), gestão de usuários,
vínculos e sessões, emissão/revogação/rotação de chaves de API, contagem de bloqueios
por cota, sincronização controlada de índices IPCA (BCB/IBGE) em `economic_indices` /
`index_points` / `index_sync_runs` e auditoria pesquisável e paginada. Migration 000004
aplicada. Detalhes em ADMIN.md.

Próxima etapa de negócio: Sprint 5, portal de autoatendimento do cliente (`/portal`).
Dívida jurídica explícita: séries curadas aplicadas ao motor, regimes legais/SELIC,
pagamentos cronológicos, feriados por tribunal e laudos homologados. Também faltam
consulta OAB, paginação integral das fontes, visualização de peças e sync agendado.
Nenhum dado legado foi importado. Cobrança externa, SMTP, RLS, retenção/LGPD,
backup/restore e operação pública permanecem futuros. Serviços legados preservados.

Commits usam identidade técnica por comando; configuração Git global preservada.
