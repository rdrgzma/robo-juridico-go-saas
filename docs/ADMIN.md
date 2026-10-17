# Painel Administrativo Global (/admin)

O painel administrativo global da plataforma (`/admin`) está implementado em Go e `html/template`, reutilizando os serviços transacionais de identidade (`internal/auth`), espaço jurídico (`internal/juridico`) e banco PostgreSQL (`internal/admin`).

## Autorização e Isolamento

- Todas as rotas sob `/admin` exigem sessão autenticada com atribuição global `platform_admin = true` e `!principal.RequiresMFA()`.
- Administradores ou proprietários (`owner`/`admin`) de organizações que não possuam `platform_admin = true` recebem `HTTP 403` em todas as rotas e métodos do pacote `internal/admin`.
- A política de MFA é preservada integralmente: apenas quando `AUTH_DISABLE_MFA=true` no ambiente local de desenvolvimento (`APP_ENV=development` em loopback com banco `rj_saas_dev`) a etapa de código TOTP é dispensada sem marcar `MFAVerified=true`.

## Módulos Entregues (Sprint 4)

1. **Dashboard administrativo (`GET /admin`)**
   - Organizações ativas e total, usuários ativos e vínculos (`memberships`) ativos.
   - Consultas (`cnj`, `indices`) e cálculos (`liquidacao`, `judicial`, `financiamento`, `prazos`) no mês corrente e total acumulado.
   - Consumo mensal compartilhado (API + painel), reservas `pending`, concluídas `completed`, estornadas `refunded` e bloqueios por esgotamento de cota (`quota_periods.blocked`).
   - Receita recorrente contratada estimada (MRR) calculada em `decimal.Decimal` (`NUMERIC(12,2)`) a partir dos contratos vigentes (`effective_from` / `effective_until`) de organizações e planos ativos, sem simular gateway de cobrança.
   - Indicadores honestos de laudos oficiais homologados (`0`) e leads captados (`0`), além de pontos de índices sincronizados.
   - Telemetria HTTP do processo (total de requisições, erros 4xx/5xx, latência média e máxima em ms) e estado operacional das integrações (PostgreSQL, DataJud/CNJ, DJEN/Comunica e BCB/IBGE).

2. **Gestão de clientes e organizações (`/admin/organizations`, `/admin/organizations/{id}`)**
   - Pesquisa e filtro por texto (nome, slug, razão social, documento, e-mail do responsável), estado (`active`/`suspended`) e plano.
   - Criação de cliente/organização com razão social, CNPJ/CPF, responsável, observações, proprietário inicial, plano, valor contratado em decimal, franquia mensal, limite por minuto (RPM) e vigência.
   - Edição de dados cadastrais, ativação/suspensão com revogação imediata das sessões ativas da organização e bloqueio instantâneo na API e no painel.
   - Atualização contratual transacional com gravação em `organization_contract_history` e auditoria em `audit_events`.

3. **Planos e autorização de recursos (`/admin/plans`)**
   - Planos cadastrados em `plans` (`padrao`, `essencial`, `escritorio` e planos customizados) com recursos por módulo (`cnj`, `judicial`, `liquidacao`, `financiamento`, `prazos`, `indices`, `api`).
   - Aplicação estrita no backend:
     - No painel (`/app`), `juridico.Service.Run` valida na mesma transação `FOR UPDATE` se o plano está ativo, se o módulo executado pertence a `plans.features` e se a data atual (`America/Sao_Paulo`) está dentro da vigência contratual (`effective_from` .. `effective_until`).
     - Na API (`/v1` e `/api/v1`), `auth.Service.BeginAPI` valida se o plano possui o recurso `'api'`, se o módulo correspondente à rota está habilitado no plano e se o contrato está vigente.

4. **Usuários e acesso (`/admin/users`)**
   - Listagem e busca de usuários com estado da conta, atribuição `platform_admin`, estado de MFA, contagem de sessões ativas e vínculos por organização.
   - Provisionamento de usuário e vínculo organizacional (`owner`, `admin`, `lawyer`, `viewer`), alteração de papel/estado com trava contra remoção do último `owner` ativo da organização ou do último `platform_admin` ativo da plataforma.
   - Revogação imediata de sessões e geração de instrução de recuperação de senha no outbox criptografado local.

5. **Chaves de API e cotas (`/admin/keys`)**
   - Visão consolidada por organização de consumo mensal, franquia, RPM, eventos concluídos, reservas pendentes, estornos e bloqueios.
   - Emissão de chave com exibição única do segredo bruto, revogação imediata e rotação transacional (`RotateKey` / `AdminRotateKey`) que revoga a chave anterior e emite uma nova preservando organização e escopos.

6. **Gestão jurídica, integrações, índices e auditoria (`/admin/operations`, `/admin/integrations`, `/admin/indices/sync`, `/admin/audit`)**
   - Configuração dinâmica das integrações públicas oficiais: o administrador global pode informar ou atualizar a chave pública oficial do DataJud (CNJ) e habilitar/desabilitar as consultas públicas ao DJEN / Comunica (`comunicaapi.pje.jus.br`) diretamente no painel.
   - Persistência das configurações na tabela `system_settings` (`datajud_api_key` e `djen_enabled`) com registro em `audit_events` (`admin.integrations_updated`).
   - Atualização a quente no motor de consultas `juridico.Sources` em memória sem necessidade de reinicializar a aplicação ou os contêineres Docker.
   - Histórico global de consultas e cálculos com filtro por organização e módulo.
   - Sincronização controlada de séries mensais do IPCA (`BCB` / `IBGE`) com validação de lacunas, persistência em `economic_indices` e `index_points` e histórico de execução em `index_sync_runs` (sem desbloquear automaticamente regimes legais não homologados no motor de cálculo).
   - Auditoria pesquisável e paginada por cursor (`before`).
