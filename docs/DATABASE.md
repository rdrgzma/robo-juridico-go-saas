# PostgreSQL

Implementado via pgx v5 e SQL explícito. Compose cria banco exclusivo rj_saas_dev
com role rj_app e banco rj_saas_test com role rj_test. Bootstrap separado; nenhuma
porta PostgreSQL publicada; nenhuma credencial ou volume do legado é utilizado.

Migration 000001_foundation.sql: organizations, economic_indices e index_points.
IDs UUID, datas timestamptz, taxas/números-índice numeric, fonte e estado verified.
Nenhum dado oficial ou de demonstração é inserido automaticamente. O catálogo não
alimenta ainda o engine HTTP: essa ligação depende da curadoria de séries futuras.

schema_migrations registra nome, SHA-256 e instante. Runner serializado por advisory
lock; SQL e ledger em transação por arquivo. Checksum alterado/arquivo ausente geram
erro. Falha faz rollback inclusive do DDL; reexecução é idempotente. cmd/migrate up
aplica; status relata. Readiness verifica conexão e checksum sem executar migrations.

Testes criam schemas de nomes aleatórios somente em rj_saas_test e removem apenas
esses schemas. URL deve ter database=rj_saas_test e user=rj_test; caso contrário,
suíte recusa. Sem TEST_DATABASE_URL os testes de integração são marcados SKIP; a
execução oficial no Compose fornece a URL e efetivamente os executa. A role de teste
não possui CONNECT em rj_saas_dev.

Migration 000002_identity.sql: usuários, memberships, papéis/permissões, sessões,
códigos de recuperação, resets, outbox criptografado, buckets de proteção, eventos
de auditoria, chaves/escopos, limites e períodos/eventos de consumo. FKs compostas
impedem relacionar sessões ou chaves a criadores fora da organização. Reservas e
estornos são transacionais; concorrência e IDOR têm testes negativos.
RLS não está habilitado; avaliação e limites em AUTH.md. Planos comerciais,
assinaturas, cálculos/laudos persistidos e jobs de negócio são etapas futuras.

Persistência do volume foi exercitada com uma organização sintética de teste antes
e depois de reiniciar app e db novos. Nenhuma base de produção foi conectada.
