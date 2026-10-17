# Banco de dados

**Planejado, ainda não implementado.** Nenhum PostgreSQL foi criado ou conectado.
Banco futuro exclusivo `robo_juridico_go_saas`, usuário sem privilégios administrativos,
porta privada. Sem volumes ou credenciais do legado.

Sprint 2: `organizations`, `users`, `memberships`, `roles`, `permissions`,
`role_permissions`, `plans`, `clients`, `subscriptions`, `api_keys`, `quota_periods`,
`usage_events`, `sessions`, `audit_events`. Sprint 3+: `queries`, `calculations`,
`reports`, `leads`, `economic_indices`, `index_points`, jobs de sincronização.

Tabelas privadas terão `organization_id NOT NULL`, índices iniciados pela organização,
FKs compostas quando apropriado (inclusive organização no vínculo pai-filho), UUIDs,
datas `timestamptz`, valores `numeric` de precisão definida ou centavos inteiros,
segredos apenas em hash e metadados JSONB validados. Índices oficiais globais não
precisam pertencer a cliente; personalizações serão privadas e versionadas.

Cotas: bloquear linha do período em transação, reservar com identificador idempotente,
evitar cobrança dupla e reconciliar reservas interrompidas. Não usar contador em memória
para franquia contratual. Período de competência precisa de fuso explícito São Paulo.

RLS é defesa adicional planejada: `FORCE ROW LEVEL SECURITY`, role da aplicação sem
`BYPASSRLS` e contexto via `SET LOCAL` em transação; jamais sessão global no pool.
Administração global deve usar caminho e privilégios separados e auditados.
Testar conexão reutilizada entre organizações e tentativa de referência cruzada.

Migrations SQL versionadas com lock e registro de checksum; alterações destrutivas
exigem plano de rollback e backup. O diretório migrations nesta sprint só documenta
essa pendência: não há DDL aprovado nem comando migrate funcional.
