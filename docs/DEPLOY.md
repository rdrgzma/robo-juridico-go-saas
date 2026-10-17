# Operação e deploy

**Não pronto para produção.** Nenhuma VPS, domínio, Nginx, systemd, firewall ou banco
existente foi acessado ou alterado. Não foi realizado deploy público.

Sprint 1 gera binário Linux/amd64 estático com recursos web embutidos. O programa
exige listener loopback e chave local. `scripts/build.sh` e `scripts/build.ps1`
compilam, mas não instalam nada. `/saude` verifica apenas processo HTTP.

Plano de Sprint 6: Ubuntu LTS em VPS 4 vCPU/8 GB/80 GB+, usuário dedicado sem login,
diretórios exclusivos `/opt/robo-juridico-go-saas/releases/<versao>` e estado fora
do diretório de releases; PostgreSQL dedicado privado. Segredos separados de fonte,
systemd com NoNewPrivileges/ProtectSystem/PrivateTmp, bind local atrás de Nginx HTTPS,
proxy trust explícito. Não usar bancos, volumes ou serviços do legado.

Instalação futura deverá validar checksums, permissões e conectividade, aplicar SQL
com lock, iniciar serviço e testar readiness. Atualização por release + symlink
atômico; rollback para release anterior só se schema compatível. Backup antes de
migration e restauração em homologação para alterações incompatíveis.

Backup externo criptografado com retenção, teste periódico de restore, rotação de
logs, alertas de disponibilidade/erros/duração, sincronização de índices e recuperação
de jobs serão requisitos de aceite. Scripts de install/update/backup/restore,
configuração Nginx/systemd e Compose opcional ainda não existem nesta sprint.
