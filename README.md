# Robô Jurídico Go SaaS

Projeto independente iniciado em 17/10/2026. **Entrega inicial da Sprint 1, ainda
parcial:** núcleo executável e testes; não é uma plataforma SaaS pronta para produção.
Backend exclusivamente Go. Nenhum código PHP, banco SQLite, HWID, dado de cliente ou
dependência de execução do legado foi incorporado.

## Executar localmente

Requer Go 1.26+; validado localmente com Go 1.27.2 Windows/amd64.
O arquivo `.env.example` é uma referência: o programa não carrega `.env` automaticamente.

```powershell
go mod download
go run ./cmd/keygen
# Copie apenas o hash impresso para a variável abaixo; guarde a chave separadamente.
$env:API_KEY_SHA256 = '<hash gerado>'
go run ./cmd/server
```

Em Linux/macOS: `export API_KEY_SHA256='<hash gerado>'`, depois `go run ./cmd/server`.
Abra `http://127.0.0.1:8080`. A escuta é restrita a IPs de loopback nesta etapa.

```powershell
$headers = @{ 'X-API-Key' = '<chave gerada>' }
Invoke-RestMethod http://127.0.0.1:8080/v1/cnj/0000001-73.2023.8.26.0100 -Headers $headers
$body = '{"principal":"1000","fator_correcao":"1.1","taxa_mensal":"1","meses":2,"pagamento":"100","imputar_juros_primeiro":true,"multa_percentual":"10","honorarios_percentual":"10"}'
Invoke-RestMethod http://127.0.0.1:8080/v1/simulacoes/liquidacao -Method Post -Headers $headers -ContentType application/json -Body $body
```

O exemplo retorna total `1226.40`; é uma fixture aritmética sintética, sem pressupor
aplicabilidade de regras legais a um processo real.

## Disponível

- Servidor Chi, encerramento gracioso, timeouts, logs JSON sem conteúdo jurídico.
- CNJ módulo 97 com rejeição estrita de caracteres e formatos inválidos.
- Liquidação parametrizada: fator explícito, juros simples/compostos em meses inteiros,
  um pagamento na data final, imputação configurável, multa e honorários sem cascata.
- Tabelas PRICE/SAC decimais, saldo zerado e última prestação reconciliada.
- Prazos por calendário informado, publicação em diário, dias úteis/corridos,
  dobra, suspensão configurável e memória diária.
- Fatores mensais de índices com fonte obrigatória e rejeição de lacunas/duplicatas.
- Dois prefixos REST, chave local obrigatória, limite global local de 60 chamadas/minuto,
  validação JSON, OpenAPI e página de estado com layout responsivo claro/escuro automático.
- CI Linux com race detector e análise de vulnerabilidades; script de build Linux.

`/app`, `/admin` e `/portal` retornam **503 módulo pendente**. Não há autenticação web,
PostgreSQL, isolamento multiempresa, cotas contratuais, integrações DataJud/DJEN,
importação/exportação, certificação ou deploy público nesta entrega.

## Verificar

```sh
go fmt ./...
go mod verify
go vet ./...
go test ./...
go build ./cmd/server
go test -race ./... # exige CGO e compilador C compatível
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
```

`scripts/build.sh` ou `scripts/build.ps1` gera `bin/robo-juridico` para Linux/amd64.
Ver evidências efetivamente executadas em [TESTING](docs/TESTING.md).

## Documentação e continuidade

- [Auditoria do legado e matriz de migração](docs/AUDITORIA_LEGADO.md)
- [Estado real e pendências](docs/PROGRESS.md)
- [Arquitetura](docs/ARCHITECTURE.md), [banco](docs/DATABASE.md), [segurança](docs/SECURITY.md)
- [API e diferenças de contrato](docs/API.md), [OpenAPI](web/openapi.json)
- [Migração](docs/MIGRATION.md), [deploy](docs/DEPLOY.md), [seis sprints](docs/ROADMAP.md)

## Novo remoto GitHub

Git inicializado em `main`, sem histórico compartilhado, submodule ou subtree. O CLI
GitHub não está autenticado e não há ferramenta de criação de repositório disponível
na conexão desta sessão; nenhum remoto foi criado nem houve push. Depois de autenticar:

```sh
gh auth login
gh repo create rdrgzma/robo-juridico-go-saas --private --source=. --remote=origin
git push -u origin main
```

Se o repositório já existir, inspecione-o antes de configurar o remoto ou publicar.
Não adicionar como remoto o projeto `robo-juridico-api`.

## Origem e licença

Análise somente leitura do commit `12685bbe7a5be048fcaf01332f67503a09b0bee8` do
repositório original. A licença MIT encontrada pertence ao subdiretório Python;
não foi presumida sua extensão aos projetos Go. Esta entrega usa implementações
novas das operações selecionadas, sem copiar os motores Go ou os recursos visuais.
O transplante desses arquivos requer comprovar permissão/licença aplicável e preservar
os avisos. Nenhuma licença de distribuição do novo projeto foi escolhida pelo agente.
