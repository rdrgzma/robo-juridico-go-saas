# Migrations executáveis

000001_foundation.sql cria a fundação PostgreSQL. Fontes SQL embutidas no binário.
Use docker compose exec app go run ./cmd/migrate up ou status. Cada arquivo possui
checksum e transação; runner usa advisory lock. Falha retorna exit 1 e não é ocultada.
Não editar arquivo aplicado. Testes usam exclusivamente o banco rj_saas_test.
