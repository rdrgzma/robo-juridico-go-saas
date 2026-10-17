# Migração futura

Nenhum banco legado foi lido, copiado ou importado nesta entrega. A fundação PostgreSQL nova está em uso local pelo Docker. Não existe ferramenta de
importação funcional; será construída após mapear o schema real de uma cópia autorizada.

Procedimento previsto:

1. Obter snapshot SQLite consistente pela API de backup ou procedimento equivalente
   autorizado; copiar só o `.db` em uso WAL não garante consistência.
2. Abrir snapshot como somente leitura e executar integrity_check; registrar hash
   do arquivo, versão do schema e contagens sem expor dados pessoais nos logs.
3. Mapear cliente legado -> organização, usuário, plano, chaves, uso, histórico,
   cálculos e laudos; guardar tabela de IDs para preservar relacionamentos.
4. Converter datas para timestamptz com fuso explícito; converter valores por decimal,
   sem float; preservar hashes de tokens compatíveis sem conhecer seus segredos.
   Hashes de senha incompatíveis exigem transição segura/recuperação, não rehash
   do hash como se fosse senha. Certificações legadas não viram novas assinaturas.
5. Dry-run em PostgreSQL descartável: validação de contagens por organização,
   totais monetários, FKs, unicidade e referências sem proprietário.
6. Importação idempotente por lote em transação, relatório de rejeições e aprovação
   de homologação antes de cutover. Nunca escrever na origem.
7. Backup do banco novo antes de importar. Rollback por restauração do banco novo
   ou exclusão controlada do lote, ensaiado em ambiente segregado. Não sobrescrever
   o legado nem migrar produção sem autorização explícita.

Ainda faltam amostras anonimizadas e schemas reais autorizados. Não há suposições
de que os modelos Laravel e o SQLite desktop sejam idênticos.
