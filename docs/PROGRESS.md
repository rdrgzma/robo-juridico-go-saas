# Progresso real

Atualização: 17/10/2026. **Sprint 1 parcial; aplicação SaaS ainda não concluída.**

## Executado

- Origem analisada em leitura no commit `12685bbe7a5be048fcaf01332f67503a09b0bee8`.
- Auditoria de módulos, segurança, contratos e inventário de duplicações: 49 pares,
  26 blobs idênticos. Não alterado o projeto original.
- Novo Git independente em main e módulo próprio; nenhum remoto/push.
- Sem identidade humana no Git local; commits desta entrega usam autor técnico
  explícito `Codex (local automation) <codex@localhost>` apenas por comando, sem
  alterar configuração global do usuário.
- Servidor Chi compilável, templates/estáticos embutidos, health e OpenAPI.
- Implementações novas limitadas de CNJ, aritmética de liquidação, PRICE/SAC,
  contagem por calendário e fator de séries completas; regressões sintéticas.
- API local com dois prefixos, hash de chave, validação, limites e logs JSON.
- CI com checks, race e govulncheck; build Linux e documentação própria.
- Rodada local final: fmt, verificação de módulos, vet, testes com cobertura e builds
  Windows/Linux passaram. Smoke do binário real passou. Govulncheck v1.9.0 não
  encontrou vulnerabilidades conhecidas. Detalhes e limites em TESTING.md.

## Pendências que impedem declarar Sprint 1 integralmente concluída

- Licença/permissão aplicável aos módulos Go e UI do legado não demonstrada;
  não foram copiados. A MIT localizada está no subprojeto Python.
- Não foram portados todos os motores, integrações DataJud/DJEN, memória completa,
  cronologia de pagamentos, juros compensatórios/legais, GAUSS e planilhas.
- Faltam fixtures de fontes oficiais e compatibilidade dos contratos antigos.
- `gh` sem login e ferramenta conectada sem operação de criar repositório: projeto
  somente local, sem repositório GitHub criado. Instruções no README.
- Race detector local depende de CGO/compilador C ausentes; workflow Linux configurado
  mas não executado no GitHub. Não considerar CI remoto aprovado.

## Etapas futuras

PostgreSQL, migrations, sessões, MFA, RBAC/organizações, franquias transacionais e
isolamento ainda não existem. Painéis retornam 503; não são interfaces operacionais.
Não há consultas externas, histórico, certificação, migração SQLite nem deploy.

Esta entrega não cumpre ainda os 14 critérios finais do produto. Cumpre a fundação
independente e uma parte testada do núcleo. Lista de arquivos no Git; comandos e
resultados efetivos em TESTING.md. Próximo trabalho: fechar lacunas do núcleo e,
na Sprint 2, introduzir banco e identidade com testes negativos de isolamento.

## Arquivos criados

| Grupo | Arquivos |
|---|---|
| Raiz/configuração | `README.md`, `go.mod`, `go.sum`, `.gitignore`, `.gitattributes`, `.env.example` |
| Comandos | `cmd/server/main.go`, `cmd/keygen/main.go` |
| Domínio | `internal/cnj/cnj.go`, `internal/calculator/calculator.go`, `internal/calculator/financing.go`, `internal/prazos/prazos.go`, `internal/indices/series.go` |
| Infraestrutura | `internal/config/config.go`, `internal/transport/http.go` |
| Testes | `internal/cnj/cnj_test.go`, `internal/calculator/calculator_test.go`, `internal/prazos/prazos_test.go`, `internal/indices/series_test.go`, `internal/config/config_test.go`, `internal/transport/http_test.go` |
| Web | `web/embed.go`, `web/openapi.json`, `web/templates/public/index.html`, `web/static/css/style.css` |
| Build/CI | `scripts/build.sh`, `scripts/build.ps1`, `.github/workflows/ci.yml` |
| Operação futura | `deploy/README.md`, `migrations/README.md` |
| Documentação | `docs/AUDITORIA_LEGADO.md`, `docs/legacy-manifest.json`, `docs/ARCHITECTURE.md`, `docs/DATABASE.md`, `docs/SECURITY.md`, `docs/API.md`, `docs/MIGRATION.md`, `docs/DEPLOY.md`, `docs/ROADMAP.md`, `docs/TESTING.md`, `docs/PROGRESS.md` |

Binários locais ignorados pelo Git: `bin/robo-juridico.exe` (Windows) e
`bin/robo-juridico` (Linux/amd64). Recursos web próprios embutidos; nenhum arquivo
precisa ser buscado no repositório original para executar esses binários.
