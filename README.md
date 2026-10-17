# Robô Jurídico Go SaaS

Sprint 1: fundação independente, ambiente Docker Compose, PostgreSQL e consolidação
inicial do núcleo Go. Painéis operacionais, autenticação multiempresa e homologação
jurídica completa serão entregues nas sprints seguintes. Não publicar em produção.

## Iniciar no Windows

Pré-requisito único de execução: Docker Desktop com engine Linux/WSL2 em funcionamento.
Não é necessário Go, PostgreSQL ou Nginx no Windows. No diretório deste repositório:

```powershell
./scripts/dev.ps1 init
# Gera .env e .dev-api-key aleatórios, sem exibi-los e sem sobrescrever existentes.
docker compose up -d --build
docker compose ps
```

WSL2/Linux: `bash scripts/dev.sh init` e os mesmos comandos Docker Compose.
Depois da primeira inicialização, não repita `init`; os segredos já existem.

**Endereço autorizado: http://localhost:8081**. O usuário escolheu a porta 8081
porque 8080 já pertence ao painel legado. Dentro do contêiner o Go escuta 8080;
no host o Compose publica exclusivamente `127.0.0.1:8081`. Banco sem porta publicada.

| Endereço local | Situação |
|---|---|
| `/` | Página de estado da fundação |
| `/docs` e `/openapi.json` | Documentação e contrato REST |
| `/health/live` | Processo HTTP ativo |
| `/health/ready` | PostgreSQL acessível e migrations corretas |
| `/v1` e `/api/v1` | API autenticada por chave |
| `/app`, `/admin`, `/portal` | 503 explícito; módulos das próximas sprints |

A chave para chamadas locais fica em `.dev-api-key`, ignorada pelo Git. Envie-a em
`X-API-Key` ou `Authorization: Bearer`. Nunca use senha como chave nem a coloque em URL.

## Operar exclusivamente por contêiner

```sh
docker compose logs -f app
docker compose exec app go run ./cmd/migrate up
docker compose exec app go run ./cmd/migrate status
docker compose exec app go test ./...
docker compose exec app go test -race -count=1 ./...
docker compose exec app go vet ./...
docker compose exec app go run ./cmd/devverify
docker build -f docker/production/Dockerfile -t robo-juridico-go-saas:local .
docker compose down
```

`down` preserva os volumes; não usar `down -v` para operação cotidiana. Air aplica
migrations antes de iniciar/reiniciar o servidor e falha explicitamente se uma delas
falhar. Alterações Go, HTML, CSS, JS e SQL são observadas por polling; recursos web
são recompilados e basta atualizar o navegador. Não há live reload de página por websocket.

Go 1.27.2, Air v1.67.4 e PostgreSQL 17.11 estão fixados. Volumes separados para dados,
módulos e cache de build. `db` é o hostname PostgreSQL; não `localhost` no contêiner.
O banco `rj_saas_test` e sua role exclusiva impedem a suíte de usar o banco de desenvolvimento.

## Núcleo consolidado

- CNJ estrito módulo 97; calendário de prazos explícito com memória.
- Liquidação, modelos/aliases do legado, juros, correção e encargos em um pacote Go.
- Financiamento PRICE/SAC/GAUSS com uma única tabela decimal compartilhada por APIs.
- Importação CSV/XLSX com limites; exportação CSV/XLSX e proteção de fórmulas CSV.
- Regressões exatas, caracterização do legado e rejeições dos comportamentos inseguros.

A API `/calculos/judicial` está habilitada para cálculo convencional com
`SEM_CORRECAO`. Não carrega seeds do legado como índices oficiais. TAXA_LEGAL,
LEI_14905 e SELIC são bloqueados na API até integrar e homologar séries oficiais.
Pagamentos intermediários são rejeitados; imputação disponível para pagamento na
data final. Esses limites fazem parte do contrato, não são correções silenciosas.
Consultas DataJud/DJEN, séries oficiais, histórico e laudos ainda serão integrados.

## Evidências e documentação

[Desenvolvimento](docs/DEVELOPMENT.md), [testes executados](docs/TESTING.md),
[progresso](docs/PROGRESS.md), [auditoria](docs/AUDITORIA_LEGADO.md),
[arquitetura](docs/ARCHITECTURE.md), [banco](docs/DATABASE.md),
[segurança](docs/SECURITY.md), [API](docs/API.md), [migração](docs/MIGRATION.md),
[deploy](docs/DEPLOY.md) e [roadmap](docs/ROADMAP.md).

Repositório Git independente em main; origem auditada em
`12685bbe7a5be048fcaf01332f67503a09b0bee8`. O titular autorizou expressamente o
reaproveitamento dos fontes nesta conversa em 17/10/2026; ver [proveniência](NOTICE.md).
Nenhum serviço, banco ou arquivo do sistema antigo foi modificado.

O remoto GitHub continua pendente de autenticação; nenhum push foi realizado.
Após autenticar e verificar que o destino não existe:

```sh
gh repo create rdrgzma/robo-juridico-go-saas --private --source=. --remote=origin
git push -u origin main
```
