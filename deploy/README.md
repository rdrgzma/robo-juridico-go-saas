# Artefatos de Operação e Deploy (Sprint 6)

Este diretório contém os artefatos preparados e testados para implantação em VPS Ubuntu LTS e a alternativa conteinerizada para produção:

## Estrutura de Arquivos

```
deploy/
├── compose.prod.yaml          # Alternativa Docker Compose para produção (sem Air, runtime não-root)
├── firewall/
│   └── ufw-setup.sh           # Regras de firewall UFW (apenas portas 22, 80 e 443 liberadas)
├── nginx/
│   └── robo-juridico.conf     # Configuração Nginx com TLS, rate limiting e cabeçalhos de segurança
├── scripts/
│   ├── install.sh             # Provisionamento inicial de usuário, diretórios e serviços na VPS
│   ├── update.sh              # Atualização atômica de versão com health check e rollback automático
│   ├── backup.sh              # Rotina de backup lógico PostgreSQL com gzip, SHA-256 e rotação
│   └── restore.sh             # Rotina de restauração com ensaio prévio em banco temporário
└── systemd/
    ├── robo-juridico.service         # Unidade de serviço systemd para o executável server
    └── robo-juridico-migrate.service # Unidade de migração de banco pré-inicialização
```

Para mais detalhes e procedimentos operacionais, consulte:
- `docs/OPERATIONS.md`: Guia operacional de produção, monitoramento e checklist de go-live.
- `docs/DEPLOY.md`: Arquitetura de compilação, requisitos de infraestrutura e variáveis de ambiente.
