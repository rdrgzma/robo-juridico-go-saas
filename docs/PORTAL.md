# Portal do Cliente (/portal)

O Portal do Cliente (`/portal`) entrega autoatendimento operacional e governança para cada organização contratante, implementado nativamente em Go e `html/template` sobre os serviços transacionais da plataforma (`internal/portal`, `internal/auth` e `internal/juridico`).

## Isolamento Multi-tenant e Segurança

- Todas as consultas e operações no portal são estritamente isoladas pelo `organization_id` da sessão ativa (`WHERE organization_id = $1`).
- Tentativas de acesso a recursos de outra organização via manipulação de identificadores em URLs, rotas ou formulários (IDOR) retornam invariavelmente `HTTP 404 (Not Found)` ou `HTTP 403 (Forbidden)`.
- Chaves de API, segredos brutos e hashes de sessão nunca são expostos em respostas HTML posteriores: o segredo de uma chave de API é exibido exclusivamente no momento da emissão ou rotação.
- Sessões ativas são gerenciadas e revogadas por identificadores opacos derivados (`SHA-256` da hash do token), impedindo vazamento de tokens de sessão no HTML.

## Módulos Entregues (Sprint 5)

1. **Dashboard do Cliente (`GET /portal`)**
   - Nome, identificadores cadastrais (CNPJ/CPF, razão social, e-mail do responsável) e situação da organização.
   - Plano contratado, vigência contratual (`effective_from` .. `effective_until`), status de ativação e recursos habilitados no plano.
   - Franquia mensal contratada, total consumido no mês, saldo disponível (`MonthlyLimit - MonthUsed`, nunca negativo), bloqueios por esgotamento de franquia e limite de requisições por minuto (RPM).
   - Resumo das operações próprias no mês: consultas CNJ/índices, cálculos de liquidação, judiciais, financiamento e prazos, memórias registradas e indicador honesto de laudos oficiais homologados (`0`).
   - Resumo do consumo de API próprio: chamadas concluídas (`completed`), pendentes (`pending`), estornadas por falha no backend (`refunded`) e chamadas gratuitas (`0` unidades faturáveis).
   - Acesso rápido às últimas 5 operações jurídicas da organização.

2. **Histórico Unificado de Utilização e Consumo (`GET /portal/usage`)**
   - Linha do tempo unificada de eventos de API (`usage_events`) e operações do painel (`legal_operations`).
   - Registro de data/hora precisa, módulo (`cnj`, `liquidacao`, `judicial`, `financiamento`, `prazos`, `indices`, `api`), identificação do ator (prefixo da chave de API ou e-mail do usuário), estado da operação (`completed`, `pending`, `refunded`), status HTTP e unidades debitadas da franquia (`1` para operações tarifadas, `0` para gratuitas ou estornadas).
   - Filtros por fonte (`all`, `api`, `panel`), por estado (`all`, `completed`, `pending`, `refunded`, `free`) e paginação transparente baseada em cursor cronológico (`before`).

3. **Chaves de API e Documentação de Integração (`/portal/keys`)**
   - Verificação dupla de autorização no servidor:
     - Permissão de papel: exige `keys:manage` (`owner` ou `admin` da organização; `lawyer` e `viewer` recebem `HTTP 403`).
     - Direito no plano contratado: exige que o plano possua `'api' = ANY(features)` e contrato ativo; organizações em planos sem API visualizam aviso explícito e têm emissão bloqueada no backend.
   - Listagem de chaves ativas e revogadas da organização com prefixo público (`rj_XXXXXXXX`), escopos autorizados (`calculations:run`, `account:read`), data de criação, último uso e estado.
   - Emissão de chave (`POST /portal/keys`): gera segredo bruto com prefixo `rj_` e exibe aviso de segurança obrigatório com botão de cópia.
   - Rotação de chave (`POST /portal/keys/{id}/rotate`): transação atômica que revoga a chave anterior e emite uma nova chave com os mesmos escopos e nome, exibindo o novo segredo apenas uma vez.
   - Revogação imediata (`POST /portal/keys/{id}/revoke`).
   - Guia prático com exemplos reais em `curl` para os endpoints `/api/v1/simulacoes/liquidacao`, `/api/v1/simulacoes/prazos` e `/api/v1/processos/cnj/{numero}`.

4. **Histórico Jurídico e Documentos (`/portal/history`)**
   - Listagem de todas as operações jurídicas e memórias de cálculo da própria organização com filtro por módulo (`liquidacao`, `judicial`, `financiamento`, `prazos`, `cnj`, `indices`) e paginação por cursor composto (`created_at` e `id`).
   - Visualização detalhada de memória (`GET /portal/history/{id}`): assinatura HMAC de integridade do laudo, entrada e resultado formatados em JSON legível.
   - Downloads protegidos (`GET /portal/history/{id}/download/{format}`): exportação em formatos oficiais (`csv`, `xlsx`, `json` com HMAC de integridade e `html`) com isolamento multi-tenant garantido no nível do banco.
   - Recálculo autorizado (`GET /portal/history/{id}/recalculate`): valida permissão `calculations:run` e redireciona com segurança para a interface de recálculo com os parâmetros preservados.

5. **Conta, Equipe e Configurações (`/portal/team`)**
   - Configurações cadastrais da organização: edição de nome fantasia, razão social, documento (CNPJ/CPF), nome e e-mail de contato do responsável legal (exige permissão `members:manage`).
   - Gestão de membros da equipe: listagem completa de colaboradores com e-mail, papel (`owner`, `admin`, `lawyer`, `viewer`), estado e data de adesão.
   - Convidar / provisionar novo membro (`POST /portal/members`): definição de papel inicial e credencial.
   - Alteração de papel (`POST /portal/members/{id}/role`): promoção ou rebaixamento de papel com proteção contra rebaixamento do último `owner` ativo.
   - Ativação / desativação de membro (`POST /portal/members/{id}/status`): suspensão imediata de acesso que revoga automaticamente todas as sessões ativas do membro suspenso na organização.

6. **Sessões, Senha e Segurança (`/portal/security`)**
   - Alteração segura de senha (`POST /portal/security/password`): verificação criptográfica da senha atual com Argon2id, incremento atômico de `password_version`, revogação automática de todas as outras sessões existentes e rotação da sessão atual com emissão de novo cookie seguro.
   - Lista de sessões ativas: histórico de sessões com indicação da sessão atual (`Current`), data de criação, último acesso e endereço IP.
   - Revogação pontual de sessão (`POST /portal/security/sessions/{id}/revoke`): encerramento de dispositivo individual sem interromper as demais sessões.
   - Encerramento global (`POST /portal/security/sessions/revoke-all`): revogação de todas as sessões do usuário com redirecionamento para o login.
   - Status e configuração de autenticação de dois fatores (MFA/TOTP).
