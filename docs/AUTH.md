# Identidade e acesso — Sprint 2

Use http://localhost:8081/login. A origem canônica é PUBLIC_ORIGIN; trocar localhost
por 127.0.0.1 nos formulários exige configurar a origem correspondente.

## Primeiro acesso local

```powershell
./scripts/dev.ps1 extend
docker compose up -d --build --wait
docker compose exec app go run ./cmd/identity bootstrap
```

O bootstrap já foi executado neste ambiente. Organização, e-mail e senha aleatórios
estão somente em `.local/bootstrap.json`, ignorado pelo Git e pelo contexto Docker.
Reexecutar preserva a conta existente. O administrador global configura TOTP no
primeiro acesso; guarde os dez códigos de recuperação mostrados uma única vez.

Para provisionar outro usuário local, sem redefinir contas existentes:

```powershell
docker compose exec app go run ./cmd/identity member -organization SEU_SLUG -email usuario@example.test -role lawyer
```

O comando informa o arquivo local das credenciais. Provisionamento é exclusivo de
desenvolvimento, exige rj_saas_dev/rj_app e não integra a imagem runtime.

## Permissões e sessões

Papéis owner, admin, lawyer e viewer pertencem à organização. Owner/admin gerenciam
membros e chaves segundo permissões; admin não promove ninguém a owner. A última
propriedade ativa é preservada mesmo sob alterações concorrentes. Viewer não calcula.
Platform_admin é atribuição global separada: owner/admin de organização não recebem
acesso ao /admin global. MFA é obrigatório para administradores globais e, após
inscrição, para qualquer usuário que o ativou. Os demais podem ativá-lo na conta.

Senhas Argon2id: 64 MiB, três iterações, paralelismo um, salt aleatório. Senhas têm
12 a 128 bytes. Até quatro operações Argon2 simultâneas por processo. Sessão opaca
aleatória, somente hash SHA-256 no PostgreSQL; validade absoluta 12h e inatividade
2h. Sessão pendente de MFA dura 5min. A conclusão do MFA rotaciona o token. Logout
revoga a sessão; encerrar todas e redefinir senha revogam as demais. Estado ativo,
papéis e versão da senha são reavaliados no banco.

Cookies HttpOnly, SameSite=Strict, host-only e Path=/. Secure acompanha origem HTTPS;
HTTP local é a exceção explícita de desenvolvimento. Formulários exigem CSRF,
Origin e Host exatos e têm corpo limitado. Segredos TOTP são AES-256-GCM; passos
TOTP repetidos e códigos de recuperação reutilizados são recusados.

## Recuperação local

A resposta de recuperação é genérica e limitada por IP/conta. Token aleatório de
15min, hash no banco, uso único; redefinição invalida sessões e preserva MFA. O link
fica criptografado no outbox local; não há envio SMTP nesta sprint.

```powershell
docker compose exec app go run ./cmd/identity outbox
```

Abra o arquivo `.local/outbox-*.json` informado pelo comando. Trate-o como segredo.
AUTH_ENCRYPTION_KEY protege MFA/outbox e deriva proteções CSRF: preserve-a, nunca
regenere como atualização rotineira. Rotação com migração dos dados é trabalho futuro.

## Chaves e cotas

Em /account/keys, owner/admin emitem e revogam chaves da organização. O valor aparece
uma vez; o banco guarda hash e prefixo. Escopos: account:read e calculations:run.
Além do escopo, a permissão atual do criador e os estados ativos são exigidos.
Cookies web não autenticam a API. /v1/me e /api/v1/me identificam a chave utilizada.
A antiga .dev-api-key só permite rotas de cálculo/CNJ sem estado, nunca /me.

Limites iniciais por organização: 1000 operações tarifáveis/mês e 60 chamadas/minuto,
compartilhados entre chaves. O mês segue America/Sao_Paulo. POST de cálculo reserva
cota em transação; somente sucesso 2xx confirma consumo. Falhas devolvem a reserva
idempotentemente. Reconciliador por minuto libera pendências com mais de dez minutos.
CNJ e consultas gratuitas registram uso sem débito mensal. Não há cobrança financeira,
catálogo comercial de planos ou interface de alteração de limites nesta entrega.

## Isolamento e limites

Filtros explícitos por organização, permissões e FKs compostas protegem memberships,
sessões e chaves; testes negativos cobrem IDOR e concorrência. RLS foi avaliado como
defesa adicional e não está habilitado: descoberta inicial de identidade exige desenho
separado de credenciais e contexto transacional SET LOCAL seguro para o pool.
Auditoria persistente registra ações de identidade. /app, /admin e /portal têm controle
de acesso, mas seus painéis operacionais completos ficam nas sprints seguintes.
Produção permanece bloqueada até homologação operacional e jurídica.
