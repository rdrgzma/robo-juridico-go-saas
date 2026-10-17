# Estado real do desenvolvimento

Atualizado em 17/10/2026. Sprints 1 e 2 entregues localmente. Plataforma completa e
produção ainda pendentes. Repositório independente com origin em
https://github.com/rdrgzma/robo-juridico-go-saas.git; legado preservado.

Sprint 1: auditoria e reaproveitamento autorizado dos motores, Docker Compose,
PostgreSQL separado, migrations, regressões jurídicas, health checks e hot reload.

Sprint 2: usuários/memberships, RBAC, sessões, Argon2id, CSRF/Origin, recuperação
local criptografada, MFA e códigos de recuperação, gestão de membros/chaves, auditoria,
cotas transacionais e reconciliação. Migration 000002 aplicada. Bootstrap local gerado
sem senha fixa; instruções em AUTH.md. /app, /admin e /portal exigem autorização e
apresentam a estrutura de acesso; ainda não são painéis operacionais completos.

Vet, testes completos com race detector, integração PostgreSQL, build Linux,
scanner de dependências e smoke/hot reload passaram dentro do Docker. Scanner sem
símbolos vulneráveis alcançáveis; advisory de módulo OpenPGP não importado descrito
em TESTING.md. Imagem runtime construída. Login conferido no navegador; fluxos de
MFA e formulários exercitados em HTTPS nos testes. Resultado local não certifica CI
remoto, pentest nem homologação jurídica.

Próxima Sprint 3: painel jurídico, fontes DataJud/DJEN, séries oficiais e cálculos
cronológicos, histórico e laudos com regressões e fontes verificáveis. Taxas legais,
SELIC e pagamentos intermediários continuam bloqueados na API até homologação.
Nenhum dado legado foi importado. Planos/cobrança, SMTP, RLS, retenção/LGPD,
backup/restore e operação pública permanecem futuros. Serviços legados preservados.

Commits usam identidade técnica por comando; configuração Git global preservada.
