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

Sprint 2 implementa Argon2id, sessões opacas revogáveis, CSRF/Origin, MFA TOTP e
códigos de recuperação (com dispensa local reversível `AUTH_DISABLE_MFA=true` restrita
exclusivamente a `development`, origem loopback e `rj_saas_dev`/`rj_app`, preservando
segredos e mantendo `mfa_verified=false`), recuperação local, RBAC por organização,
chaves com escopos, cotas e auditoria.
Detalhes, exceção Secure no HTTP local e limites em AUTH.md. Isolamento por filtros
de organização e FKs compostas testados; RLS não habilitado. Limites de plano são
fundação SQL, sem cobrança comercial. APP_ENV=production continua bloqueado até
homologação operacional, proteção de downloads, retenção/LGPD, backup/restore,
entrega externa de e-mail e revisão de segurança. Build não equivale a deploy.

Para laudos: cálculo no servidor, proveniência e versão do motor, resultado integral
canônico assinado/HMAC com segredo externo e rotação; frontend jamais prova autenticidade.
Nenhum desses controles futuros é considerado concluído por existir página ou tabela.

Painel Sprint 3: histórico/downloads/recálculo filtrados por organização, com permissão
atual e política de MFA ativa; imports limitados, resultados escapados e CSV protegido. Memória privada
com HMAC do registro completo e CSS próprio com hash CSP na exportação HTML.
Sem verificador público, uploads de laudo ou alegação de certificação jurídica.
Fontes possuem destinos fixos, timeout, limite de resposta e não seguem redirects.
Nenhuma chave ou corpo de consulta é registrado nos logs. Detalhes em WORKSPACE.md.
