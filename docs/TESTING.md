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
Compose, mas não foi executado remotamente, pois ainda não houve publicação no GitHub.

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
