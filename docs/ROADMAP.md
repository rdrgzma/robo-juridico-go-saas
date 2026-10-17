# Seis sprints

1. Fundação: Git independente, auditoria, Docker Compose/PostgreSQL, migrations,
   hot reload, health checks, motores consolidados e regressão. Entrega desta sprint.
2. Autenticação e multiempresa: usuários/memberships, papéis/permissões, sessões,
   recuperação, MFA administrativo, testes de isolamento e cotas transacionais.
3. Painel jurídico: adaptação autorizada da UI desktop, consultas DataJud/DJEN,
   séries oficiais e curadoria de cálculos, pagamentos cronológicos, histórico,
   calendário por tribunal, laudos e exportação segundo permissões.
4. Admin: clientes/planos/contratos, usuários/chaves/cotas, auditoria e indicadores.
5. Portal: plano/consumo/chaves, histórico/laudos, documentação e configurações.
6. Homologação e operação: carga/segurança/backup/restore, Nginx/systemd/VPS e alternativa Docker.

A Sprint 1 não significa paridade jurídica completa nem SaaS pronto. Os módulos
reaproveitados estão centralizados e testados; políticas não homologadas são bloqueadas
na API. Caracterizações com seed sintético não substituem fixtures oficiais.
Persistência e infraestrutura antecipadas para a Sprint 1 pela instrução adicional.
Não iniciar refatoração jurídica ampla sem regressões específicas e fontes verificáveis.
