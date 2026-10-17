# API REST — Sprint 1

Fonte de contrato: `web/openapi.json`, servida em `/openapi.json`. Saúde: `GET /saude`.
`production_ready:false` é intencional; health é liveness e não readiness PostgreSQL.

Nos dois prefixos `/v1` e `/api/v1`:

| Método e sufixo | Comportamento |
|---|---|
| GET `/cnj/{numero}` | Valida formato e módulo 97, retorna metadados básicos |
| POST `/simulacoes/liquidacao` | Aritmética parametrizada; strings decimais obrigatórias |
| POST `/simulacoes/financiamento` | Tabela PRICE/SAC; principal em centavos exatos |
| POST `/simulacoes/prazos` | Calendário informado, sem seleção automática de tribunal |

Autenticação: `X-API-Key` ou `Authorization: Bearer <chave>`. Se ambos forem enviados,
X-API-Key tem precedência. Chave local única; não há scopes, usuários ou organizações
nesta sprint. Não usar cookies como autenticação de API. Limite compartilhado de
60 tentativas/minuto por processo; `429` com `Retry-After:60`.

Erros: `{"erro":{"codigo":"...","mensagem":"..."},"request_id":"..."}`.
`400` JSON inválido/campos desconhecidos/corpo acima de 64 KiB; `401` chave inválida;
`415` content type; `422` entrada fora dos limites; `404` rota; `405` método.

Liquidação arredonda principal corrigido, juros e encargos a centavos. Juros sobre
principal corrigido por meses inteiros; um pagamento apenas, na data final. Se
`imputar_juros_primeiro` for falso, aplica primeiro ao principal. Encargos incidem
sobre o saldo após pagamento, sem encargo sobre encargo. Pagamento excedente é
rejeitado; fator/valores negativos são rejeitados. `compostos` ausente = falso;
`meses` ausente = zero. Não há inferência de taxas, datas ou índices oficiais.

Financiamento calcula tabela única; não compara contrato com mercado. Sistemas
PRICE/SAC apenas, com taxa zero permitida explicitamente. A última prestação é
ajustada junto à amortização. Não caracteriza cobrança indevida nem gera laudo.

Prazos: `data_evento` ISO, `dias` 1..365, `modo` UTEIS/CORRIDOS, `intimacao`
DIRETA/DIARIO. DIARIO publica no primeiro dia útil seguinte. Exclui o evento ou
publicação, inclui vencimento; vencimento sem expediente prorroga. Feriados devem
ser fornecidos; fins de semana são excluídos dos úteis. Suspensão 20/dez..20/jan
somente quando solicitada. Não há catálogo CPC/CLT/tribunal aprovado.

## Diferenças do legado

Preservados os prefixos e autenticação por cabeçalho. Não foi declarada paridade
de todos os endpoints. As rotas novas `/simulacoes/*` evitam confundir contratos
reduzidos com `/calculos/judicial`, `/calculos/financiamento` e `/prazos/calcular`.
Essas rotas antigas e processos/OAB/webhooks/conta/índices/importações ainda retornam
404; não são redirecionadas silenciosamente.

CNJ mantém o caminho mas está explicitamente em desenvolvimento: retorna
`numero_original`, `formatado`, `digitos`, `valido`, `ano`, `segmento`,
`tribunal_codigo`. O legado HTTP usa `ok`, `numero`, `numero_digitos`, tribunal e
outros metadados; CNJ inválido era 200 e agora é 422. Não substituir a API antiga
com este binário. O adapter compatível e seus testes ficam pendentes.

`/app`, `/admin`, `/portal`: 503 com erro de módulo pendente. A página raiz é apenas
estado técnico do projeto, não dashboard de operação jurídica.
