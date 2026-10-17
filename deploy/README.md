# Preparação de implantação

Sprint 1 produz binário Linux, mas bloqueia escuta fora de loopback. Não instalar em
produção nem publicar via Nginx: a autenticação de organizações ainda não existe.

Os arquivos Nginx, systemd, instalação, atualização, backup/restauração e rollback
serão entregues e testados na Sprint 6 com PostgreSQL e armazenamento definidos.
Nenhum serviço, banco, volume ou diretório do legado pode ser utilizado.
