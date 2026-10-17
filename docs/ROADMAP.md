# Roadmap de seis sprints

| Sprint | Entrega | Estado em 17/10/2026 |
|---|---|---|
| 1 — Fundação/núcleo | Auditoria, Git independente, Go executável, domínio e regressão | Parcial: base executável pronta; porte completo dos motores e fontes, licença e fixtures oficiais pendentes |
| 2 — Banco/auth/tenancy | pgx, migrations, organização/usuários/papéis, sessões, MFA e isolamento | Não iniciada |
| 3 — Painel jurídico | Identidade desktop adaptada, consultas/fontes, cálculos completos, prazos, índices, histórico PostgreSQL, laudos/exportações conforme permissões | Não iniciada |
| 4 — Admin | Clientes/planos/usuários/chaves/cotas, auditoria, indicadores/leads/laudos | Não iniciada |
| 5 — Portal | Plano/consumo/chaves/histórico/laudos/documentação/configurações | Não iniciada |
| 6 — Homologação/deploy | Integração, segurança, carga, backup/restore, Linux, Nginx/systemd, operação | Não iniciada |

Antes de encerrar integralmente Sprint 1: comprovar permissão para reuso Go/HTML,
acrescentar testes de regressão com séries oficiais e contratos do legado, portar
fontes com rede simulada, validar taxa legal/transição e pagamentos cronológicos.
Não iniciar refatoração ampla sem os testes correspondentes.

Próxima sequência técnica: concluir essas lacunas em fatias testadas; então banco
e identidade multiempresa. Não ativar os painéis antes de testar isolamento.
