# API — Sprint 2

Host local autorizado: http://localhost:8081. Contrato completo em /openapi.json;
página em /docs. Prefixos /v1 e /api/v1 equivalentes. API exige X-API-Key ou Bearer;
X-API-Key tem precedência. Chaves de organização com escopos; cookies web não autenticam esta API.

| Sufixo | Método | Função |
|---|---|---|
| /cnj/{numero} | GET | Formato estrito e módulo 97 |
| /simulacoes/liquidacao | POST | Fator explícito, meses inteiros, pagamento final |
| /simulacoes/financiamento | POST | Tabela PRICE/SAC/GAUSS compartilhada |
| /simulacoes/prazos | POST | Calendário informado, memória diária |
| /calculos/judicial | POST | Contrato Go reaproveitado, correção e juros convencionais |
| /calculos/financiamento | POST | Contrato revisional e aliases do legado |
| /calculos/indices | GET | Catálogo habilitado, inicialmente SEM_CORRECAO |

Root do prefixo retorna versão/link da documentação após autenticação. /saude é
mantido; /health/live independe do banco, /health/ready verifica conexão e checksums.
Os painéis /app, /admin e /portal exigem autenticação; funções operacionais futuras.

Erros: {"erro":{"codigo":"...","mensagem":"..."},"request_id":"..."}.
400 para JSON inválido/campo extra/mais de 64 KiB; 401 chave; 415 media type;
422 parâmetros, decimais fora dos limites ou política não homologada; 429 com
Retry-After para 60 chamadas/minuto globais locais. Não equivale a cota comercial.

## Limites e diferenças explícitas

Decimais de simulação em strings, ponto e até 8 casas; contratos do legado aceitam
representação decimal do DTO, mas rejeitam precisão/expoentes extremos. Valores de
saída são strings. Nunca comparar dinheiro em float64.

A liquidação indexada só habilita SEM_CORRECAO nesta sprint: não há séries oficiais
carregadas automaticamente. TAXA_LEGAL, LEI_14905 e SELIC retornam 422 na API até
curadoria e regressão oficiais. O núcleo herdado permanece disponível em biblioteca
para testes de caracterização, sem alegação de homologação desses regimes.

Pagamentos apenas na data final. Pagamento histórico retorna erro, evitando a
aproximação antiga que não recalculava os eventos intermediários. Prioridade do
art. 354 do contrato herdado está limitada à mora e principal e explicitada nos campos
imputados; casos mais amplos dependem do motor cronológico. Ordenação dos itens não
muda o pagamento final. Entrada de valor superior ao saldo suportado é rejeitada.

Juros convencionais respeitam taxa zero. Meses comerciais usam dias limitados a 30;
capitalização composta usa potência decimal nos meses inteiros e interpolação linear
na fração residual. Aplicabilidade e convenção devem ser confirmadas no caso real.
Encargos seguem o contrato informado, sem inferir se são juridicamente devidos.

Financiamento: Finance gera uma única tabela para API nova e wrapper revisional;
última prestação ajustada com amortização. Taxa revisada zero é zero deliberado,
não vira contratada automaticamente. Aliases de parcelas/taxas foram preservados,
mas a distinção entre campo omitido e zero ainda exige consumidor explícito. O valor
comparativo não prova cobrança indevida nem obrigação de restituição.

CNJ mantém caminho e parser estrito, porém os campos ainda são os metadados básicos
formatado/digitos/ano/segmento/tribunal_codigo; inválido é 422, diferentemente do
handler legado 200/valido:false. Adapter integral aguarda a integração processual.

CSV/XLSX são serviços Go testados; seus endpoints de upload/exportação completos
serão ligados ao painel com permissões nas sprints seguintes. Também não existem
nesta sprint processos/OAB/webhooks/fontes externas e certificação de laudos.
Não substituir a API antiga com este binário como se houvesse paridade total.

## Identidade e consumo

GET /v1/me (ou /api/v1/me) exige chave de organização e account:read; retorna
user_id, organization_id, organization, email, role, platform_admin, mfa_verified,
mfa_enabled e permissions. Flags MFA descrevem o principal da chave, não uma sessão
web autenticada. .dev-api-key é somente compatibilidade para rotas sem estado e
não permite /me. Para cálculos, exige calculations:run e permissão atual do criador.

Chaves emitidas/revogadas em /account/keys. Erros adicionais: 403 escopo/permissão,
402 cota mensal, 429 proteção por IP ou limite por organização. Defaults: 1000 POSTs
de cálculo bem-sucedidos/mês (America/Sao_Paulo), 60 chamadas/minuto compartilhadas.
Reserva antes de calcular, estorno em falhas; pendências >10min são reconciliadas.
CNJ/consultas gratuitas registram uso sem débito mensal. Detalhes em AUTH.md.
