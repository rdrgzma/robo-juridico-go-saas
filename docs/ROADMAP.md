# Seis sprints

1. Fundação: Git independente, auditoria, Docker Compose/PostgreSQL, migrations,
   hot reload, health checks, motores consolidados e regressão. Concluída.
2. Autenticação e multiempresa: usuários/memberships, papéis/permissões, sessões,
   recuperação, MFA administrativo, testes de isolamento e cotas transacionais. Concluída; painéis operacionais nas próximas etapas.
3. Painel jurídico: entrega funcional com UI inspirada no desktop, fontes opcionais
   DataJud/DJEN, cálculos convencionais, prazos, consulta IPCA BCB/IBGE, histórico,
   recálculo e exportações privadas. Limites e dívidas em WORKSPACE.md: homologação
   das fontes, motor cronológico, políticas legais, calendário por tribunal, sync
   agendado, laudos públicos e visualização de peças não estão concluídos.
4. Admin: clientes/planos/contratos, usuários/chaves/cotas, auditoria e indicadores.
5. Portal: plano/consumo/chaves, histórico/laudos, documentação e configurações.
6. Homologação e operação: carga/segurança/backup/restore, Nginx/systemd/VPS e alternativa Docker.

A Sprint 1 não significa paridade jurídica completa nem SaaS pronto. Os módulos
reaproveitados estão centralizados e testados; políticas não homologadas são bloqueadas
na API. Caracterizações com seed sintético não substituem fixtures oficiais.
Persistência e infraestrutura antecipadas para a Sprint 1 pela instrução adicional.
Não iniciar refatoração jurídica ampla sem regressões específicas e fontes verificáveis.
