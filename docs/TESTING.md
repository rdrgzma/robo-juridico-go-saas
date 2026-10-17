# Evidências da Sprint 1

Data: 17/10/2026. Windows com Docker Desktop Linux/WSL2, Docker Engine 29.8.2 e
Compose v5.6.0. Ambiente oficial Go 1.27.2 em contêiner, PostgreSQL 17.11 e Air v1.67.4.
Os testes locais da primeira entrega sem Docker são históricos; esta rodada os substitui.

## Comandos efetivamente executados

| Verificação | Resultado |
|---|---|
| Inicializador por contêiner | .env/.dev-api-key gerados sem exibir segredos |
| docker compose config --quiet | Configuração válida |
| docker compose up -d --build | Imagem app criada e dois serviços inicializados; porta 8081 autorizada |
| docker compose ps | app/db saudáveis, somente app publicado em 127.0.0.1:8081 |
| docker compose exec -T app go run ./cmd/migrate up | 000001_foundation.sql aplicada, repetição idempotente |
| docker compose exec -T app go run ./cmd/migrate status | Migration aplicada |
| go fmt ./... no app | Fontes formatadas |
| go mod verify no app | all modules verified |
| go vet ./... no app | Exit 0 |
| go test -race -count=1 ./... no app | Todos os pacotes passaram, inclusive PostgreSQL; sem falhas do race detector |
| go build no app | Binário Linux compilado |
| scripts/build.ps1 | Compilação Linux executada no app, sem Go no host |
| Dockerfile multi-stage | Imagem runtime construída; id confirmou uid=65532 gid=65532 |
| cmd/devverify no app | Smoke HTTP, cálculo conhecido 1020.00, reload Go/HTML/CSS e restauração passaram |
| Reinicialização do projeto novo | Organização sintética permaneceu no banco com volume nomeado |
| Navegador | Página raiz e /docs renderizadas em localhost:8081 |
| Govulncheck v1.9.0 | Após corrigir x/text para v0.41.0: No vulnerabilities found. |

Race detector que não executava no Windows na primeira entrega agora foi executado
com sucesso dentro do Linux Docker. CI está configurado com os mesmos comandos por
Compose. Esta evidência da Sprint 1 antecede a publicação; resultados remotos não foram confirmados.

## Testes significativos

CNJ com fixtures exatas e entradas malformadas; juros simples/compostos e taxa zero;
liquidação conhecida, aliases, pagamento final e invariância da ordem; lacunas de
correção e juros produzem erro; pro rata comercial no mesmo mês; PRICE/SAC/GAUSS com
conservação do principal, juros não negativos e última prestação reconciliada.

Importação: CSV/TSV, XLSX com inline strings/células esparsas, rejeição de data/valor
inválidos, arquivo excessivo e ZIP expandido excessivo; exportação CSV/XLSX e proteção
contra fórmulas em descrição. Prazos: diário/direta, úteis/corridos, prorrogação,
feriados informados, dobra e suspensão. HTTP: chave, prefixos, corpo/decimais limitados,
campos extras, rate limit, painéis bloqueados, health e indisponibilidade de dependência.

PostgreSQL: schema vazio, reaplicação, rollback de SQL inválido sem DDL parcial nem
ledger falso, checksum adulterado, runners concorrentes serializados, proibição de
conexão da role de testes ao banco de desenvolvimento. Testes criam/removem apenas
schemas aleatórios no banco rj_saas_test; não apagam tabelas do banco principal.
Readiness real também foi exercitada com pool apontando para porta inacessível,
sem interromper banco real; liveness permanece 200 e readiness retorna 503.

O registro de persistência é sintético: UUID 00000000-0000-4000-8000-000000000101,
nome Fixture de persistência Sprint 1. Sua presença após reinicialização foi consultada.
Horários de início dos contêineres legados foram conferidos e permaneceram iguais
antes/depois da reinicialização restrita ao novo projeto.

## Falhas encontradas e corrigidas durante o trabalho

- Porta 8080 em uso pelo legado: usuário autorizou 8081; nenhum contêiner antigo foi parado.
- Importação inicial dos fontes apresentou mistura de finais de linha/encoding:
  normalizada para UTF-8/LF antes da compilação final.
- Regressão encontrou taxa convencional zero substituída por 1%, corrigida.
- GAUSS com taxa zero podia gerar juro negativo por arredondamento, corrigido.
- Probe HTML usava comentário removido por html/template; alterado para marcador
  oculto, confirmando recompilação e resposta efetiva, com restauração posterior.
- Scanner detectou GO-2026-6629 e GO-2026-5970 no x/text indireto; atualização e nova
  varredura concluídas com sucesso. Air também recebe a versão corrigida da dependência.

## Limites da evidência

Seed herdado é somente caracterização sintética; pagamentos dos testes herdados foram
adaptados à data final permitida. Não comprova série oficial ou equivalência universal
com contas antigas. Comandos de servidor/CLI e web têm smoke de processo real, não
cobertura unitária completa. Storage é exercitado pelos testes de integração, embora
a cobertura por pacote padrão o apresente como zero quando medido em outro pacote.

Ainda não testados/implementados: login/CSRF/MFA, isolamento SaaS/IDOR/RLS, fontes reais,
curadoria jurídica completa, carga, backup/restore, migração SQLite e deploy VPS.
Não confundir zero vulnerabilidades conhecidas com homologação de segurança ou jurídica.

## Sprint 2 — execução em 17/10/2026

`go fmt ./...`, `go vet ./...`, `go test -race -count=1 ./...` e build do servidor
executados no app Docker: exit 0. Integração PostgreSQL com race: 46,068s. Ambas as
migrations aplicadas. cmd/devverify passou: cálculo 1020, reload Go/HTML/CSS e restauração.
Imagem runtime construída; login real renderizado no navegador. Fluxos completos
HTTPS via httptest verificam cookies, CSRF/Origin, MFA, logout e separação cookie/API.

Testes adicionais: Argon2/salts/limites, AES/AAD/adulteração, vetores RFC6238,
expiração absoluta/inatividade, reset único/expirado, revogação global, isolamento de
membros/chaves, suspensão, downgrade de permissões, último owner concorrente,
limite de login persistente, teto mensal concorrente, estorno idempotente,
reconciliação, limite compartilhado e virada do mês em São Paulo.

Govulncheck v1.9.0: exit 0, nenhum símbolo vulnerável ou pacote vulnerável importado.
Existe advisory de módulo GO-2026-5932 para golang.org/x/crypto/openpgp, pacote
não importado pela aplicação. x/crypto v0.58.0 é usado para Argon2id; x/text v0.43.0.
Isso não equivale a ausência de todo advisory de módulo nem substitui pentest.
CI está configurado; resultados remotos não foram confirmados nesta entrega.

## Atalhos locais — 17/10/2026

Testes com race para carregador e integração dos cinco perfis passaram. Sem arquivo/
fora do ambiente e banco de desenvolvimento, atalhos desativados; rota ausente.
Senhas ausentes no HTML, CSRF inválido e Fetch Metadata cross-site recusados, papéis
preservados e administrador global redirecionado ao MFA. Testes HTTPS de login/MFA
existentes e go vet passaram. No navegador interno, os cinco botões foram conferidos
e o atalho Advogado abriu a conta com papel lawyer. Compatibilidade Origin null
restrita ao desenvolvimento loopback habilitado documentada em AUTH.md.

## Sprint 3 — painel jurídico, 17/10/2026

Execução em Docker: go fmt, go vet, go test -race -count=1 ./... e build Linux
passaram. Integração completa: 94,789s nesta rodada, com build da imagem em paralelo.
Imagem multi-stage construída. Scanner v1.9.0: zero vulnerabilidades em símbolos ou
pacotes importados; permanece o advisory de módulo OpenPGP não utilizado já descrito.
Não foi confirmado resultado remoto do GitHub Actions.

Testes novos de workspace: cálculo conhecido 1020.00, persistência entre instâncias,
idempotência concorrente, rejeição de conteúdo diferente na mesma chave, quota
concorrente exata, rollback em entrada inválida, revogação de permissão, histórico
entre organizações, downloads e recálculo com ID de outra organização, XSS, CSRF,
importação multipart CSV real, recálculo de parcelas recuperadas no servidor e XLSX.
HMAC é estável após normalização JSONB e recusa adulteração do resultado completo.
Migrations de banco vazio/reaplicação/concorrência atualizadas para três arquivos.

Clientes externos: contratos sintéticos DataJud/DJEN, cache separado por organização,
404 tratado como falha, processo divergente, respostas excessivas, séries BCB/IBGE,
lacunas/duplicação/valores ausentes e limite de doze meses. TestOfficialIndexSmoke foi
executado com RJ_EXTERNAL_SMOKE=1 e passou nas APIs reais BCB/IBGE: janeiro 2024 =
0.42%, fevereiro = 0.83%. Sem rede na suíte padrão. DataJud/DJEN não consultados ao vivo.

Navegador local: login de advogado, /app, formulário de revisão com exemplo sintético,
resultado persistido (1066.19 contratado, 1052.77 revisado, diferença 13.42), e consulta
real IPCA BCB com histórico conferidos. Registros locais de smoke estão identificados
como Teste local; não são dados de cliente. Limites funcionais em WORKSPACE.md.

Verificação final: cmd/devverify passou com cálculo 1020.00 e reload/restauração de
Go/HTML/CSS. Runtime confirmou uid/gid 65532 e ausência de /src e .local. Status das
três migrations aplicado, docker compose config --quiet e git diff --check válidos.

## Sprint 4 — painel administrativo (/admin), 17/10/2026

Execução em Docker: migration `000004_admin.sql` aplicada (`schema_migrations` com 4
arquivos), `go fmt ./...`, `go vet ./...`, `go test -race -count=1 ./...` (integração
PostgreSQL em 71,828s) e `cmd/devverify` passaram com exit 0.

Testes novos em `tests/integration/admin_test.go`:
- Negação (`403` / `ErrDenied`) para `owner` ou `admin` de organização que não possua `platform_admin = true`.
- Cadastro e atualização de planos com recursos granulares (`cnj`, `prazos`, `liquidacao`, `api`) e aplicação imediata no backend (`/app` e `/api/v1`).
- Criação de organização/cliente com razão social, CNPJ, responsável, valor contratado em decimal, vigência e registro em `organization_contract_history`.
- Contabilização de bloqueios por esgotamento de cota mensal (`quota_periods.blocked`) tanto na API (`HTTP 402`) quanto no painel (`ErrQuota`).
- Emissão, rotação atômica (`RotateKey` / `AdminRotateKey` revogando a chave anterior e retornando a nova chave apenas uma vez) e revogação de chaves de API.
- Suspensão de organização revogando imediatamente sessões ativas e bloqueando o acesso.
- Gestão de usuários e vínculos com proteção contra remoção do último `owner` da organização ou do último `platform_admin` ativo da plataforma, e disparo de recuperação no outbox criptografado.
- Sincronização controlada de índices IPCA (`SyncIndices`) com validação de lacunas e gravação em `economic_indices`, `index_points` e `index_sync_runs`.
- Pesquisa e paginação da trilha de auditoria (`ListAuditEvents`) e renderização autenticada de todas as rotas HTTP de `/admin`.

