# Arquitetura

Monólito modular Go. cmd/server compõe configuração, pool pgx e transporte Chi.
Domínio em internal/calculator/cnj/prazos/indices, sem dependência da interface.
web usa html/template, embed e CSS responsivo. cmd/migrate é independente do servidor;
migrations SQL embutidas também no binário. PostgreSQL oficial desde a Sprint 1.

A instrução Docker de 17/10/2026 substitui o desenvolvimento por Go instalado no
Windows: imagem Go 1.27.2 + Air v1.67.4, PostgreSQL 17.11, caches em volumes e bind
somente do repositório novo. O usuário autorizou bind host 127.0.0.1:8081, pois 8080
é do legado. Dockerfile multi-stage gera server/migrate/healthcheck estáticos e runtime
UID/GID 65532 sem toolchain ou fontes.

Consolidação: um pacote calculator, contratos reaproveitados com autorização. Finance
é a única implementação de tabelas PRICE/SAC/GAUSS usada pelos dois contratos HTTP.
Não há cópia desktop ou dependência do repositório antigo. Seed legado existe somente
em *_test.go. Cálculos com índices oficiais exigem integração e homologação futura;
a API bloqueia taxas legais ainda não validadas, em vez de substituir valores ausentes.

Liquidação simples por fator informado permanece como simulação distinta da interface
indexada; não certifica fator nem resultado. A convenção de meses comerciais e fração
linear residual está explícita; não há pressuposto de aplicabilidade universal.
Pagamentos históricos precisam de motor cronológico e são rejeitados nesta etapa.

internal/auth implementa identidade, sessões, MFA, recuperação, RBAC, chaves e cotas;
internal/tenancy transporta o principal explícito por contexto. cmd/identity provisiona
contas locais, sem incluir credenciais fixas ou ferramenta no runtime. Web autentica
por cookie e CSRF; API exige chave, escopo e permissão atual. PostgreSQL persiste
sessões, auditoria e reservas; o reconciliador recupera reservas abandonadas.
Painéis operacionais admin/portal/juridico serão desenvolvidos nas próximas sprints.
Produção bloqueada até homologação operacional. Ver AUTH.md para limites e RLS.
