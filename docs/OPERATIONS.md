# Guia de Operações e Prontidão de Produção

Este guia descreve as rotinas operacionais, procedimentos de contingência, monitoramento e o checklist formal para lançamento da plataforma Robô Jurídico Go SaaS em ambiente de produção (Sprint 6).

---

## 1. Rotinas de Backup e Restauração Testadas

### Política de Backup
- **Frequência**: Diária (agendada via cron às 03:00 UTC).
- **Tipo**: Backup lógico via `pg_dump` com compressão `gzip`, preservação de sequências e integridade relacional.
- **Integridade**: Cada arquivo `.sql.gz` é acompanhado de seu respectivo arquivo `.sha256` gerado imediatamente após o dump.
- **Retenção**: 7 dias locais em `/opt/robo-juridico/backups/` e replicação criptografada para armazenamento externo segregado.

### Execução de Backup Manual
```bash
# Na VPS:
sudo /opt/robo-juridico/scripts/backup.sh

# Em ambiente Docker local:
./scripts/backup.sh
```

### Procedimento de Restauração Segura (com Ensaio em Staging)
O script de restauração executa três etapas obrigatórias:
1. Validação do checksum SHA-256 do arquivo comprimido.
2. Restauração de ensaio em um banco de dados temporário descartável (`rj_staging_restore_test`) para garantir que o dump não possui corrupções sintáticas ou dados truncados.
3. Restauração definitiva no banco alvo com encerramento automático em caso de erro (`ON_ERROR_STOP=1`).

```bash
# Na VPS:
sudo /opt/robo-juridico/scripts/restore.sh /opt/robo-juridico/backups/rj_saas_prod_20261017_030000Z.sql.gz

# Em ambiente Docker local:
./scripts/restore.sh backups/rj_saas_dev_20261017_030000Z.sql.gz
```

---

## 2. Monitoramento, Telemetria e Logs

### Health Checks da Aplicação
- **Endpoint HTTP**: `GET /healthz`
  - Retorna `HTTP 200 OK` com payload JSON contendo a saúde do banco de dados PostgreSQL e dos subsistemas ativos.
  - Se o pool de conexões do PostgreSQL estiver degradado ou inacessível, o endpoint retorna `HTTP 503 Service Unavailable`.
- **Binário de Sondagem**: `/usr/local/bin/rj-healthcheck` (executado nativamente pelo Docker e pelos scripts de atualização).

### Telemetria Operacional (`/admin`)
O painel administrativo consolida em tempo real:
- Requisições HTTP totais e taxa de erros 4xx / 5xx.
- Latência média e pico em milissegundos.
- Contadores de bloqueios por esgotamento de franquia mensal (`quota_periods.blocked`).
- Estado das integrações externas (PostgreSQL, DataJud CNJ, DJEN Comunica, BCB e IBGE).

### Logs Estruturados
- A aplicação utiliza `log/slog` com saída em formato estruturado.
- Em produção sob systemd:
  ```bash
  journalctl -u robo-juridico.service -f --output=cat
  ```
- Nenhuma senha, segredo bruto de API key, token de sessão ou dado pessoal é registrado em log.

---

## 3. Rotação de Chaves e Segredos

### Chave de Criptografia de Dados (`AUTH_ENCRYPTION_KEY`)
- Utilizada para selar mensagens temporárias de outbox e payloads sensíveis em repouso.
- Para rotacionar:
  1. Gerar nova chave de 32 bytes (64 caracteres hexadecimais):
     ```bash
     go run ./cmd/keygen
     ```
  2. Atualizar no arquivo de ambiente `/opt/robo-juridico/etc/robo-juridico.env`.
  3. Reiniciar o serviço `robo-juridico.service`.

### Chaves de API de Organizações
- Disponível de forma autônoma pelo cliente em `/portal/keys` e pelo administrador em `/admin/keys`.
- A rotação revoga a chave anterior imediatamente e emite um novo segredo exibido uma única vez, preservando escopos e cotas sem interromper o histórico de auditoria.

---

## 4. Atualização de Versão sem Downtime (Zero-Downtime Rollout)

O script `deploy/scripts/update.sh` orquestra a substituição atômica:
1. Executa backup preventivo do banco de dados antes da aplicação.
2. Faz backup dos binários em execução (`server.prev` e `migrate.prev`).
3. Instala os novos binários em `/opt/robo-juridico/bin/`.
4. Executa as novas migrações via `robo-juridico-migrate.service`.
5. Reinicia o serviço principal via `systemctl restart robo-juridico.service`.
6. Realiza smoke check em `http://127.0.0.1:8080/healthz`.
7. **Rollback automático**: se o health check não responder positivamente em 2 segundos, os binários anteriores são restaurados imediatamente e o serviço é reiniciado na versão estável.

---

## 5. Checklist Pré-Lançamento (Go-Live Checklist)

Antes de alterar o status da plataforma para produção pública, os seguintes critérios devem ser verificados e aprovados:

| Item | Requisito | Status |
|---|---|:---:|
| 1 | `APP_ENV=production` configurado estritamente | [ ] |
| 2 | `AUTH_DISABLE_MFA=false` (exigência estrita de MFA fora de dev) | [ ] |
| 3 | Certificado TLS válido instalado com nota A+ no SSL Labs | [ ] |
| 4 | Firewall UFW ativo liberando exclusivamente portas 22, 80 e 443 | [ ] |
| 5 | PostgreSQL configurado com conexões exclusivas em `127.0.0.1` | [ ] |
| 6 | Senhas fortes e independentes para `rj_bootstrap` e `rj_app` | [ ] |
| 7 | Migrations `000001` a `000005` aplicadas e conferidas | [ ] |
| 8 | Rotina diária de backup agendada no cron com rotação e ensaio | [ ] |
| 9 | Domínio oficial configurado em `PUBLIC_ORIGIN` com cookies seguros | [ ] |
| 10 | Usuário do processo isolado sob UID não privilegiado (`robo-juridico`) | [ ] |
| 11 | Suíte de testes automatizados com `-race` aprovada sem falhas | [ ] |
| 12 | Homologação formal dos regimes jurídicos antes do desbloqueio em UI | [ ] |
