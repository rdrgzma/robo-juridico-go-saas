# Arquitetura

## Decisão

Monólito modular Go: `cmd/server` compõe configuração e transporte; domínio em
`internal/cnj`, `calculator`, `prazos`, `indices`; interfaces web em `web`, embutidas
no binário via embed e renderizadas por html/template. Nenhum acesso ao legado em runtime.
Chi roteia os dois prefixos. Decimal para todas as operações monetárias; representação
JSON em strings evita perda de centavos no cliente. SQL explícito/pgx será Sprint 2.

Não criar dezenas de pacotes vazios: auth/tenancy/storage/admin/portal/jurídico e
`cmd/migrate` serão introduzidos com implementação e testes. Essa é a única redução
da estrutura proposta. Um só núcleo atende futuros painéis e API.

## Limites atuais

Camadas puras não conhecem HTTP, PostgreSQL, usuários ou variáveis de ambiente.
`indices` valida séries informadas, mas ainda não está conectado à liquidação HTTP:
esta recebe um fator explícito de simulação. É proibido tratar esse fator como prova
de tabela oficial ou usar o resultado como laudo certificado.

Chave única local e limite global são mecanismos de desenvolvimento, não tenancy.
Os três painéis ficam bloqueados e o listener exige loopback. Não há estado de
cliente, jobs, caches nem persistência nesta entrega. Reiniciar zera o rate limiter.

## Próxima composição

Middleware autentica sessão/chave e resolve identidade, organização, papéis e escopos
no servidor; serviços validam plano e organização; repositórios recebem identidade
explícita e SQL parametrizado. UI jamais decide autorização. Cotas são reservadas
transacionalmente com idempotência e finalização recuperável. Fontes externas usam
contexto, prazo global, corpos limitados, hosts fixos, cache limitado e proveniência.

O roteamento web será por sessões seguras; não reaproveitar a chave local como login.
Laudos exigem cálculo confiável no servidor e artefato assinado sobre conteúdo integral.
