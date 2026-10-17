# Desenvolvimento local oficial

Docker Compose é obrigatório. Use Docker Desktop Linux/WSL2 no Windows. Todos os
comandos Go do projeto executam em contêiner. `scripts/dev.ps1` e `scripts/dev.sh`
encapsulam init/up/down/test/vet/migrate/status/logs/build.

## Primeira execução

1. No repositório novo, executar `./scripts/dev.ps1 init` ou `bash scripts/dev.sh init`.
   Um contêiner Go oficial gera quatro segredos independentes em .env e o token em
   .dev-api-key. Ambos ignorados; nunca são exibidos pelo inicializador. Não sobrescreve
   arquivos existentes. Não copiar .env do legado.
2. `docker compose up -d --build`. O banco deve ficar saudável antes do app.
3. `docker compose ps`: app e db saudáveis; abrir http://localhost:8081.

Porta 8081 autorizada pelo usuário em 17/10/2026 para preservar o painel antigo em
8080. Host bind 127.0.0.1; interno Go 0.0.0.0:8080; PostgreSQL db:5432 sem publicação.

## Ambientes e identidades

Compose fixa APP_ENV=development. .env fornece apenas segredos. O servidor exige
configuração explícita, conexão PostgreSQL e hash da chave. APP_ENV=test é utilizado
pelo comando de migrations para selecionar exclusivamente TEST_DATABASE_URL; a suíte
usa essa URL com validação de banco/role e schemas aleatórios isolados. O servidor
web não inicia em test. APP_ENV=production permanece bloqueado até a segurança SaaS
estar homologada. Não existe usuário/admin demonstrativo fixo nem seed automático.

O bootstrap PostgreSQL cria rj_app/rj_saas_dev e rj_test/rj_saas_test, sem superuser,
CREATEDB ou CREATEROLE. CONNECT público revogado. A role de testes não pode acessar o
banco de desenvolvimento. O bootstrap possui segredo independente e não é usado pelo app.

## Comandos

```sh
docker compose up -d --build
docker compose ps
docker compose logs -f app
docker compose exec app go run ./cmd/migrate up
docker compose exec app go run ./cmd/migrate status
docker compose exec app go test ./...
docker compose exec app go test -race -count=1 ./...
docker compose exec app go vet ./...
docker compose exec app go mod verify
docker compose exec app go run ./cmd/devverify
docker compose down
```

cmd/devverify modifica temporariamente Go/HTML/CSS para verificar recompilação,
resposta HTTP e restauração. Execute sem edições concorrentes nesses arquivos.
O browser deve ser atualizado manualmente após o rebuild. Não depende de Node ou
Go no host. Os exemplos HTTP podem ser enviados do cliente de sua preferência.

## Migrations e persistência

Air executa `go build ... && go run ./cmd/migrate up` antes do servidor. Arquivos SQL
embutidos em migrations/, ledger com checksum, lock PostgreSQL e uma transação por
arquivo. Falha interrompe inicialização; nunca marcar SQL inválido como aplicado.
Arquivo já aplicado não deve ser alterado: criar a próxima migration. `status` relata
aplicação; não há down destrutivo automático.

Volumes nomeados: postgres_data, go_modules e go_build, isolados pelo nome de projeto
robo-juridico-go-saas. Restart/down preservam os dados. Não apagar volumes nem mudar
credenciais em .env de banco já inicializado: editar o arquivo não altera a senha
existente. Rotação exige procedimento PostgreSQL controlado e atualização coordenada.

Healthcheck do db usa TCP localhost para evitar considerar o servidor temporário do
initdb pronto. /health/live independe do banco; /health/ready testa banco e checksums.
A porta 5432 mostrada como 5432/tcp em compose ps é interna, não porta publicada.

## Hot reload e problemas comuns

Air v1.67.4 com polling de 500 ms, debounce de 800 ms e espera de encerramento de
12 s. Binário temporário em /tmp/rj-air; não sincroniza binário Windows. Cache em
volumes separados. Pastas .git/bin/tmp/docs/docker excluídas da observação. Go/HTML/
CSS/JS/JSON/SQL recompilam os recursos embed.

Se DB estiver indisponível, corrigir a causa e reiniciar apenas o app novo. Logs não
exibem URL do banco nem token. Se 8081 já estiver ocupada, verificar quem ocupa e
mudar apenas o bind do projeto novo; não parar contêineres legados.

Primeiro build baixa imagens e Air. WSL2 com repo no filesystem Linux costuma reduzir
latência; o bind em caminho Windows com espaços também foi exercitado nesta sessão.
Não montar arquivos/volumes da aplicação antiga. `.dockerignore` exclui segredos,
.git, binários e bancos do contexto de imagem. A imagem de runtime só recebe binários.

## Identidade local (Sprint 2)

Para instalações existentes: `./scripts/dev.ps1 extend` adiciona somente a variável
AUTH_ENCRYPTION_KEY ausente; depois `docker compose up -d --build --wait` aplica a
configuração. `docker compose exec app go run ./cmd/identity bootstrap` gera conta
local em .local/bootstrap.json; preserva bootstrap existente. Acesse
http://localhost:8081/login e configure MFA. Use a origem canônica PUBLIC_ORIGIN.

Comandos member/outbox e política de segredos em AUTH.md. .local é excluído do Git,
Docker e Air. HTTP local usa cookie sem Secure; HTTPS o exige. Não regenere a chave
de criptografia existente: ela protege MFA/outbox. Não existe envio real de e-mail.
