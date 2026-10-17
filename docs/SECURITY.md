# Segurança

## Implementado e limitado ao ambiente local

- Escuta somente loopback; inicialização falha sem hash SHA-256 configurado.
- `cmd/keygen` usa 32 bytes aleatórios; hash comparado em tempo constante.
- Nenhuma chave de demonstração ou fallback no servidor. O token deve ser aleatório,
  não senha; SHA-256 é apropriado ao token de alta entropia, não ao hash de senha.
- JSON até 64 KiB, campos desconhecidos rejeitados, só um objeto, media type JSON.
- Decimais com sintaxe e tamanho limitados; limites de meses, parcelas e calendário.
- Limite global local de 60 chamadas/minuto, inclusive falhas de chave.
- Timeouts HTTP, tamanho máximo dos cabeçalhos, CSP, nosniff, frame deny, no-store,
  sem CORS permissivo e sem interpretação de X-Forwarded-For/X-Real-IP.
- Logs JSON com método/status/duração e request ID; não registrar URL/consulta,
  corpo, número de processo, IP, chave, segredo ou conteúdo jurídico.
- Nenhum upload, download privado ou armazenamento de cliente aberto.

## Pendências antes de qualquer produção

Argon2id com parâmetros aferidos, sessões opacas aleatórias com hash no banco,
expiração absoluta/inatividade, rotação de sessão, cookies Secure/HttpOnly/SameSite,
CSRF em cada mutação web, validação de Origin, login com limite e proteção contra
enumeração, recuperação com token curto e uso único e MFA administrativo.

RBAC + organização + escopos de API + plano no backend em todas as operações.
SQL parametrizado, testes negativos de IDOR e autorização inclusive exportações,
URLs assinadas curtas para download quando usadas, retenção de dados pessoais e
auditoria. Proteção contra força bruta deve distinguir IP confiável/conta e nunca
permitir que uma chave interna contorne os limites de outra organização.

Laudos: cálculo no servidor, fonte dos índices, versão do motor, inputs e resultado
integrais, serialização canônica e assinatura/HMAC com chave externa versionada.
Frontend não certifica valores; verificação pública expõe somente dados mínimos.

Importação: limite comprimido/descomprimido, quantidade de entradas/linhas/células,
tipo real de arquivo, sem extração arbitrária no filesystem; exportar strings de
usuário com proteção contra fórmulas. Integrações não aceitarão URL livre (SSRF).

Nginx/HTTPS, proxy trust restrito, segredos via ambiente protegido, banco privado,
backup externo criptografado, restore ensaiado, dependências verificadas e teste de
carga ficam nas sprints seguintes. O limite local desta sprint não resolve tudo isso.
