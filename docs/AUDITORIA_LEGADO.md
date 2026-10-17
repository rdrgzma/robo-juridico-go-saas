# Auditoria do legado

Data: 17/10/2026. Origem: https://github.com/rdrgzma/robo-juridico-api.
Commit fixado: `12685bbe7a5be048fcaf01332f67503a09b0bee8`.

## Método e limites

As seções iniciais e a matriz abaixo registram a primeira inspeção. A atualização
no fim deste arquivo descreve a autorização e a entrega posterior com Docker.


Leitura via `git ls-tree` e `git show` em clone auxiliar sem checkout. Nenhuma escrita,
branch, commit ou push no repositório original. Não foram carregados `.env`, bancos,
executáveis, credenciais nem dados pessoais. Não houve chamadas reais DataJud/DJEN,
execução do Laravel ou suíte do legado. As conclusões são de inspeção estática dos
arquivos indicados, não de pentest ou homologação jurídica.

Abreviações: **G** = `4-api-robo-juridico-php-go/api-juridica-go/`;
**D** = `5-app-juridico-desktop/`; **P** = `4-api-robo-juridico-php-go/painel/`.
Todos os caminhos de origem abaixo se referem ao commit fixado, não ao HEAD futuro.

O inventário de blobs [legacy-manifest.json](legacy-manifest.json) compara 49 pares
de arquivos Go internos presentes em G e D: **26 são byte a byte idênticos**.
`calculator/engine.go`, `models.go`, `interest.go`, `financing.go`, `correction.go`,
`repository.go`, `seed.go`, `importer.go`, `exporter.go` e seus testes correspondentes
estão entre os pares idênticos. Coletores possuem variantes. Não se deve manter dois
motores no novo monólito.

## Matriz de origem, destino, risco e estratégia

| Módulo | Implementação observada | Destino | Risco e estratégia | Estado na primeira entrega |
|---|---|---|---|---|
| CNJ | G e D `internal/cnj/cnj.go`, testes reais | `internal/cnj` | Remove qualquer não dígito; valida DV sem restringir completamente tribunal. Reescrever parser estrito; homologar mapa de tribunais antes de habilitar fontes | Novo parser/DV e fixtures sintéticas; metadados reduzidos |
| Liquidação | G/D `internal/calculator/engine.go` | `internal/calculator` | Fator 1 após erro de correção, erro de juros descartado; não transplantar silenciosamente | Novo cálculo parametrizado limitado; motor completo pendente |
| Juros e Lei 14.905 | G/D `calculator/interest.go` | políticas do calculator | Série ausente ignorada; soma mensal inclui meses de início/fim; regime anterior assume percentual padrão. Exigir fixtures de fonte oficial e política aprovada | Sem regra legal automática nesta entrega |
| Pagamentos / art. 354 | `engine.go`, ramo `ParcelaPagamentoParcial` | cronologia de eventos do calculator | Acumuladores dependem da ordem da entrada e já estão atualizados à data final. Exigir testes de pagamentos intermediários e invariância da ordenação | Apenas pagamento na data final e prioridade configurável |
| Multa/honorários | G/D `calculator/penalties.go`, chamado por engine | calculator | Bases de incidência precisam de contrato explícito; não inferir aplicabilidade legal | Percentuais informados, mesma base sem cascata |
| PRICE/SAC/GAUSS | G/D `calculator/financing.go` | calculator | PRICE converte em float64; correção final de amortização não altera prestação. Revisada <=0 vira contratada e pode esconder taxa zero deliberada | PRICE/SAC novos, decimais e reconciliação; GAUSS e revisão comparada pendentes |
| CSV/XLSX | G/D `calculator/importer.go`, `exporter.go`, testes | serviços de importação/exportação | CSV permissivo; inspeção inicial não mostrou limite de expansão ZIP; descrições CSV precisam proteção contra fórmulas | Não portados; quotas de bytes/linhas e ZIP obrigatórias |
| Prazos | G/D `internal/prazos/prazos.go` | `internal/prazos` | Rito desconhecido vira útil; feriado inválido é descartado; feriados forenses generalizados. Separar calendário por jurisdição/versionamento | Novo contador rejeita entradas inválidas, exige calendário explícito; CPC/CLT completo pendente |
| Índices | G/D `calculator/repository.go`, `seed.go` | `internal/indices` + PostgreSQL | Seed embutido não comprova série oficial; origem e revisões precisam rastreabilidade | Fator mensal puro, sem seed, com rejeição de lacunas |
| Coleta Bacen/IBGE | G `calculator/collectors/bacen.go`, `ibge.go`, `worker.go`; D variantes | índices / worker | Agendamento inicial/diário existe; persistência e consistência do lote precisam testes | Adaptação futura, nenhuma rede agendada |
| Tabelas tribunais | G `calculator/collectors/tribunais.go` | coletores homologados | Deriva tabelas de IPCA/INPC/IGPM; não demonstra coleta oficial de cada tribunal. Não rotular aproximações como tabelas oficiais | Não reaproveitado |
| DataJud | G/D `fontes/datajud/client.go`, parser e testes | `internal/juridico` / fontes | Cliente usa prazo, retentativas e chave configurável; depende de network e domínio legado. Validar cancelamento, tamanho de resposta e mapa de tribunais | Candidato à adaptação após licença; não integrado |
| DJEN | G/D `fontes/comunica/client.go`, sanitizer/testes | jurídico / fontes | 400/404 convertidos em lista vazia; distinguir ausência e falha. Remover dependência de corrida de proxies e validar URLs/documentos | Não integrado |
| Consolidação | handlers/orquestração e domínio G/D | jurídico | Dados das fontes incompletos não podem ser inventados; registrar fonte, instante e avisos | Pendente |
| Histórico | G `storage/database.go`, `sqlite.go`; D interface exibe histórico local | storage PostgreSQL | Modelo por cliente não equivale a organização + usuário + permissões | Pendente Sprint 2/3 |
| Chaves e cotas | G `auth/auth.go`, `transport/http/middleware.go`, `storage/sqlite.go` | auth/tenancy/usage | Há hash SHA-256, comparação constante, rate limiter e reserva/finalização. Limiter usa nome do cliente; finalização descarta erro | Só chave local de teste operacional, sem conta/cota contratual |
| Admin | P `Filament/Resources/{Cliente,Chave,Admin,Uso,Evento,CalculoSalvo}Resource.php`, widgets | `internal/admin` | CRUD de clientes/contratos e métricas existem; não assumir MFA/RBAC completo pela presença de painel | Reimplementação Go Sprint 4 |
| Portal | P `Filament/Portal/Resources/MinhaChaveResource.php`, `Pages/MeuPlano.php`, recursos de uso/cálculos | `internal/portal` | Query de chaves filtra `cliente_id`; senha exige atual mas mínimo observado é 6. Verificar todas as ações por organização | Reimplementação Go Sprint 5 |
| Laudos/leads | P `Services/CalculoJudicialService.php`, models/migrations | serviço de laudos | Hash parcial com sufixo constante; não autentica todo o conteúdo nem substitui cálculo no servidor | Não emitir até implementar integridade e autorização |
| Desktop UI | D `ui/static/index.html`, `style.css`, `app.js` | web templates/static | Sidebar e temas existem; licenciamento/HWID e histórico local precisam ser retirados. Não copiar JS com confiança em autorização local | Página nova de estado; painéis operacionais pendentes |
| Licenciamento | G/D `internal/licensing/licenca.go` | direitos por plano | Vinculação ao hardware inadequada ao SaaS | Não incorporado |
| Documentação | `docs/03-DOCUMENTACAO-TECNICA-E-ARQUITETURA.md`, router/OpenAPI G | docs próprias/OpenAPI | Documento geral descreve Python/FastAPI e não prova implementação Go/Filament | Documentação nova com estado verificável |

## Achados de segurança e integridade

1. **Alta prioridade — resultados incompletos com sucesso:** em `engine.go`, erro de
   `ObterFatorAcumuladoComOpcoes` vira fator 1; retorno de erro de
   `CalcularJurosMoratorios` é ignorado. Em `interest.go`, pontos indisponíveis são
   simplesmente omitidos. Nova política: falhar explicitamente se falta dado.
2. **Alta prioridade — integridade de laudo parcial:** `CalculoJudicialService.php`
   hasheia UUID, processo, data, total e quantidade de itens com um sufixo constante.
   Alterar valores internos preservando total/quantidade pode não alterar o hash.
   Usar resultado integral canônico calculado no servidor e HMAC/chave externa ao
   código ou assinatura, com versionamento e rotação.
3. **Alta prioridade — identidade de rede:** `middleware.go::ObterIP` aceita
   `X-Forwarded-For` e `X-Real-IP` de qualquer origem. Pode afetar auditoria/bloqueio.
   A nova versão não usa esses cabeçalhos; futura confiança depende do proxy conhecido.
4. **Média — permissões operacionais:** o router aplica autenticação comum à rota
   `POST /calculos/coletar`, sem verificação visível de escopo administrativo nesse
   encadeamento. Adotar escopos específicos; inspecionar todas as ações sensíveis.
5. **Média — CORS:** o middleware do legado permite `*`. A nova API não emite CORS
   permissivo; allowlist deve ser uma decisão explícita de integração futura.
6. **Média — chave fallback:** o construtor PHP possui fallback literal para chave
   de integração. Não foi copiado. Nova configuração falha sem hash configurado.
7. **Financeiro — reconciliação:** última linha PRICE/GAUSS modifica amortização e
   saldo sem recalcular a prestação. A implementação nova ajusta a prestação final,
   testando `prestação = juros + amortização` e conservação do principal.

Esses achados não significam que todo fluxo seja explorável: faltam execução dinâmica
e análise integral de configuração. Não se afirmou ausência de controles apenas por
não encontrá-los no trecho examinado.

## Contratos JSON e lacunas de negócio

- O handler CNJ legado responde `ok`, `numero`, `numero_digitos`, `valido`, tribunal
  e metadados; inválidos retornam HTTP 200 com `valido:false`. O parser interno tem
  `numero_original`, `formatado`, `digitos`. Não confundir modelo interno com resposta HTTP.
- O serviço PHP usa aliases `total_geral`/`total_geral_devido`, `itens`/
  `itens_liquidados`; modelos Go já carregam esses aliases, mas montagem precisa
  teste de preenchimento consistente.
- Financiamento aceita `numero_parcelas`/`quantidade_parcelas` e duas grafias das
  taxas. Taxa zero não pode ser confundida com campo ausente.
- Documentar erro do CNJ e contratos de simulação novos em API.md. Esta entrega
  não afirma compatibilidade binária/JSON com todos os consumidores do legado.
- Ainda precisam de fixtures verificáveis: taxa legal histórica/transição, juros
  compensatórios, imputação cronológica, múltiplos pagamentos, bases do art. 523,
  GAUSS, feriados de cada tribunal, pro rata e planilhas reais anonimizadas.

## Reuso e dependências

Reutilizáveis tecnicamente com pouco acoplamento: CNJ, modelos decimais, fórmulas
puras e alguns testes/parser/sanitizer. Adaptar: engine, séries, coletores, fontes,
cache e importadores com limites. Reimplementar: autenticação web, PostgreSQL,
tenancy/RBAC, admin/portal, cotas transacionais, laudos e operação VPS.

**Licença:** apenas `3-api-robo-juridico/api-robo-juridico/LICENSE` foi localizada,
com MIT e aviso de copyright do projeto Python. Não há licença Go demonstrada pela
árvore examinada; não extrapolar essa permissão a G ou D. Na primeira entrega não foi copiado código Go; posteriormente o titular
autorizou expressamente a reutilização, conforme atualização abaixo. A autorização de reutilização aplicável deve ser documentada antes do porte.

Dependências legadas observadas incluem Chi, shopspring/decimal, SQLite, x/time/rate,
HTTP com abstração de rede/proxies, Laravel/Filament e cliente QR. O SaaS mantém
Chi/decimal, planeja pgx e dispensa PHP/SQLite/licenciamento desktop. DataJud requer
chave própria; DJEN e séries oficiais precisam contratos e disponibilidade validados.

## Sequência de migração

1. Congelar fixtures aritméticas e registrar a referência de origem (feito).
2. Entregar núcleo independente limitado e testes (feito).
3. Resolver permissão dos fontes e acrescentar fixtures oficiais antes de transplantar
   motores/fontes; não rotular esta pendência como Sprint 1 integral concluída.
4. PostgreSQL/tenancy primeiro; depois persistência e painéis.
5. Adapters de compatibilidade com testes por endpoint; homologação integrada apenas
   com ambiente segregado e dados sintéticos/anonimizados.


## Atualização após autorização e instrução Docker — 17/10/2026

O titular declarou “Sou titular e autorizo a reutilização”. Proveniência dos fontes
copiados/adaptados registrada em NOTICE.md. A ausência de licença Go na árvore do
legado deixou de impedir o reaproveitamento autorizado para este projeto.

Motor, modelos, correção/juros/encargos, planilhas e testes foram consolidados em
internal/calculator. O seed só é compilado nos testes. O financiamento revisional e
a simulação agora compartilham uma única Finance decimal para PRICE/SAC/GAUSS.
O parser XLSX foi substituído para suportar células esparsas/inline strings, com limites
de ZIP/XML/linhas/células. CSV rejeita linhas inválidas e neutraliza texto como fórmula.

Corrigidos: erros de correção/juros propagados, zero convencional preservado, validações
antes de operações decimais extensas, datas inválidas, alias OK e indexador preenchidos,
última prestação reconciliada, correção independente do pagamento e imputação final
independente da ordem dos itens. Pagamentos históricos são rejeitados, evitando herdar
uma aproximação incorreta; TAXA_LEGAL/SELIC são bloqueadas na API até homologação oficial.

Os testes herdados receberam adaptações explícitas: repositório de caracterização
usa seed exclusivamente de teste, pagamentos foram movidos à data final permitida e
SEM_CORRECAO foi excluído da expectativa de fator >1. Não se afirma equivalência
universal com as contas antigas nem homologação de dados oficiais por esses testes.

Docker Compose é o ambiente oficial. PostgreSQL, migrations/health, banco separado
para testes e imagem non-root implementados. Porta 8081 autorizada para preservar os
contêineres legados que já usam 8080/8000; seus horários de início permaneceram iguais.
Veja TESTING.md para as verificações efetivamente realizadas e PROGRESS.md para o
limite entre esta fundação e as funcionalidades de sprints posteriores.
