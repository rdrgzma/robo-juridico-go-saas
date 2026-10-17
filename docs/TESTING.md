# Testes e evidências

Ambiente local: Windows/amd64, Go 1.27.2 obtido do site oficial com verificação
SHA-256 do arquivo. Go não estava no PATH original. Nenhum teste do legado foi
executado; não se atribuem os testes novos à suíte anterior.

## Cobertura funcional

- CNJ: fixture sintética com resto calculado independentemente por inteiro grande,
  formatos equivalentes, rejeição de lixo/Unicode/segmento inválido/DV incorreto,
  corpus de fuzz e invariantes.
- Liquidação: resultados exatos simples e compostos, taxa zero, rejeições de fator
  ausente, pagamento excedente, sinal e limites; exemplo R$ 1.226,40.
- PRICE/SAC: taxa zero, 1% e 2,85%, parcela conhecida SAC, conservação do principal,
  saldo, última prestação e identidade prestação = juros + amortização.
- Prazos: diário/direta, úteis/corridos, prorrogação, calendário local, dobra,
  recesso, data/feriado/rito inválidos.
- Índices: fator conhecido, deflação, lacuna, duplicata, proveniência ausente.
- HTTP: autenticação, equivalência entre prefixos, JSON desconhecido/extra/grande,
  rate limit, health, página raiz, painéis bloqueados e configuração fail-closed.

## Execução

O primeiro teste revelou uma fixture CNJ escrita com DV errado; o valor esperado
foi corrigido após cálculo independente (`base % 97 = 25`, DV = 73). A primeira
compilação também antecedeu a criação de openapi.json e falhou no embed; arquivo
adicionado antes da rodada de verificação final.

Rodada final executada em 17/10/2026, após atualização para Chi v5.5.0 e
shopspring/decimal v1.5.0:

| Verificação | Resultado observado |
|---|---|
| `go fmt ./...` | Executado; fontes formatadas |
| `go mod verify` | `all modules verified` |
| `go vet ./...` | Exit 0, sem diagnósticos |
| `go test -count=1 -cover ./...` | Todos os pacotes de teste passaram |
| Calculator | 92,2% dos statements |
| CNJ | 95,0% |
| Config | 91,7% |
| Índices | 85,2% |
| Prazos | 95,0% |
| Transporte HTTP | 93,1% |
| `go build -o bin/robo-juridico.exe ./cmd/server` | Binário Windows gerado, exit 0 |
| `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o bin/robo-juridico ./cmd/server` | Binário Linux gerado, exit 0; não executado em Linux |
| `go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...` | Exit 0: `No vulnerabilities found.` |
| `go test -race ./...` | Não executou os testes: `-race requires cgo`; falta compilador C compatível neste ambiente |
| Smoke do binário Windows | Processo real em 127.0.0.1:18080; health, HTML, OpenAPI, liquidação 1226.40 e rejeição 401 passaram; processo encerrado |

`cmd/keygen`, `cmd/server` e `web` não possuem testes diretos; cobertura contabilizada
nesses pacotes é zero. A página, OpenAPI e o entrypoint foram exercitados no smoke.
Não confundir workflow configurado com execução remota aprovada: CI GitHub ainda
não executado, pois não há remoto publicado. Segurança sem vulnerabilidades conhecidas
no scanner não significa homologação de produção.

## O que ainda não foi testado

Integrações reais, dados oficiais, regressão integral do legado, isolamento
PostgreSQL/IDOR/RLS, sessões/CSRF/MFA, carga, restore, instalação VPS e produção.
Esses módulos ainda não foram implementados. As fixtures são aritméticas sintéticas,
não uma homologação jurídica de todos os métodos solicitados.
