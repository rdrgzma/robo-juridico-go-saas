# Segurança

Sprint 1 local: API key aleatória de 32 bytes, apenas hash SHA-256 no processo,
comparação constante, limite global de 60 chamadas/minuto, JSON até 64 KiB, campos
extras rejeitados, limites de decimais/expoentes antes de conversão extensa, datas,
meses, parcelas e calendário. Segredos gerados por contêiner e ignorados no Git/contexto
de imagem. .env não é carregado implicitamente pelo Go; Compose injeta as variáveis.

Logs JSON sem URL de banco, chave, corpo, número de processo ou query string. Sem
confiança em X-Forwarded-For e sem CORS permissivo. Timeouts HTTP, CSP, nosniff,
frame deny, no-store. Shutdown aguarda requisições em andamento antes de fechar o pool.

PostgreSQL privado na rede do projeto; roles de app/testes sem privilégios administrativos.
Cálculos não possuem acesso direto ao banco do legado. Bind host 127.0.0.1:8081.
Imagem runtime usa UID/GID 65532. Não há upload HTTP público ainda; importadores Go
limitam 2 MiB comprimidos, 16 MiB expandidos, 100 entradas ZIP, 1000 linhas e 64 colunas;
erros não descartam linhas silenciosamente. CSV neutraliza fórmulas em texto de usuário.

Seed legado só em testes. Erro de correção/juros impede resultado; não há fallback 1.
Rotas legais não homologadas rejeitam a requisição. Nenhum resultado do frontend é
certificado; laudos ainda não são emitidos. Rate limiter local não é franquia de cliente.

Ainda obrigatório antes de produção: Argon2id, sessões opacas/expiração/rotação,
cookies Secure/HttpOnly/SameSite, CSRF/Origin, proteção login/recuperação, MFA admin,
RBAC por organização/plano/escopo, cotas transacionais, IDOR/RLS e auditoria persistente.
Também faltam proteção de downloads, retenção/LGPD e backup/restauração ensaiados.
APP_ENV=production está bloqueado até esses controles; Dockerfile é preparação de build.

Para laudos: cálculo no servidor, proveniência e versão do motor, resultado integral
canônico assinado/HMAC com segredo externo e rotação; frontend jamais prova autenticidade.
Nenhum desses controles futuros é considerado concluído por existir página ou tabela.
