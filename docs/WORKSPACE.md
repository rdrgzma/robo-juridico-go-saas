# Painel jurídico — entrega funcional da Sprint 3

Acesse http://localhost:8081/app depois de autenticar um usuário com calculations:run.
Owner, admin e lawyer possuem essa permissão; viewer continua restrito à conta/portal.
No ambiente local padrão (`AUTH_DISABLE_MFA=true`), o segundo fator fica dispensado;
com `AUTH_DISABLE_MFA=false`, o administrador global e contas com TOTP ativo precisam concluir MFA.

## Funcionalidades

- Visão geral com contagem real do histórico e consumo mensal da organização.
- Consulta CNJ com validação local e consulta opcional DataJud/DJEN.
- Liquidação judicial com juros convencionais, encargos selecionados e parcelas
  manuais ou importadas de CSV/XLSX. Correção habilitada: SEM_CORRECAO.
- Simulação alternativa por fator explícito e meses inteiros.
- Revisão de contrato PRICE comparado ao sistema revisado PRICE/SAC/GAUSS, taxa
  revisada inclusive zero. Diferença matemática não determina restituição.
- Prazos com datas, intimação, dias úteis/corridos, dobro, feriados e recesso explícitos.
- Consulta IPCA mensal BCB/SGS 433 ou IBGE/SIDRA 1737/63, até doze meses por intervalo.
- Histórico PostgreSQL por organização e usuário, paginação estável por data/UUID,
  entrada completa, resultado e versão do motor. Não há edição de registro pela aplicação.
- Recálculo abre formulário e gera nova operação. Parcelas de importações são
  recuperadas pelo servidor, com o mesmo isolamento por organização.
- CSV para resultados, XLSX para liquidação judicial, JSON com HMAC e memória HTML
  independente com CSS embutido para impressão. Downloads exigem sessão/permissão.
- Tema claro/escuro e menu responsivo inspirados na interface desktop autorizada.

## Consumo e segurança

Cálculos bem-sucedidos consomem uma unidade da mesma quota_periods usada pela API.
Consultas CNJ/índices não debitam franquia mensal. A gravação do histórico, consumo
mensal e auditoria ocorre numa transação com lock de organization_limits. Falhas de
cálculo não consomem. Envios duplicados usam identificador aleatório de idempotência;
mesmo identificador com conteúdo alterado retorna 409. O limite por minuto do painel
é por organização, separado do bucket por minuto da API; a franquia mensal é conjunta.

Formulários exigem CSRF/Origin e permissão atual. Importação multipart apenas em
/app/judicial/import, limite total de 2 MiB + 32 KiB, arquivo até 2 MiB, 1000 linhas,
limites ZIP do importador. Dados não são executados como HTML. CSV neutraliza fórmulas.
Valores e decisões ficam no servidor; frontend não fornece resultado certificado.

A memória exportada inclui HMAC-SHA256 com chave derivada de AUTH_ENCRYPTION_KEY e
separação de finalidade. O conteúdo assinado inclui organização, usuário, título,
entrada e resultado completos, versão, identificador e instante. JSON canônico usa
números exatos, sem conversão monetária para float64. A verificação privada compara
a assinatura informada ao registro persistido; não é verificador público de uploads.
A memória não se apresenta como laudo pericial homologado nem certifica aplicação legal.
Preserve a chave: rotação e verificação de documentos antigos exigem política futura.

## Fontes externas

Por padrão DATAJUD_API_KEY está vazia e DJEN_ENABLED=false. Configure somente o .env
local e recrie o app novo: `docker compose up -d --no-deps --force-recreate app`.
Não reutilize segredos do legado. Nunca envie chave no chat, Git ou URL.

A consulta CNJ só acessa rede ao marcar a opção de fontes externas. O número é enviado
às fontes oficiais habilitadas. DataJud tem aliases explícitos para justiça estadual,
federal e trabalhista; os demais ramos são identificados como não suportados.
Há limite de dez registros DataJud e primeira página com vinte publicações DJEN;
a interface indica truncamento. Não se afirma cobertura integral, partes disponíveis,
documentos ou ausência de processo a partir de resultado vazio. HTML de publicação
é preservado como dado escapado na memória, não executado ou embutido em iframe.

Timeout de seis segundos por fonte, cancelamento, resposta até 2 MiB, sem seguir
redirecionamentos nem aceitar URL arbitrária. DataJud/DJEN são consultados em paralelo.
Cache de até 200 consultas por cinco minutos, chave inclui organização e processo;
falhas não viram sucesso vazio. Metadados incluem fonte, instante e avisos específicos.
DataJud com chave e DJEN ao vivo ainda não foram homologados nesta instalação.

IPCA é consultado em endpoints fixos BCB/IBGE, com origem e coleta preservadas no
resultado. Meses ausentes, duplicados, valores especiais ou fora do intervalo geram
erro, sem preencher zero. O teste externo opt-in confirmou janeiro/fevereiro de
2024 em ambas as APIs. Não há sincronização agendada nem ativação automática dessas
séries no motor judicial; catálogo global/curadoria e políticas legais são outra etapa.

Referências técnicas consultadas em 17/10/2026:
- [API Pública DataJud — CNJ](https://www.cnj.jus.br/sistemas/datajud/api-publica/)
- [Aliases e endpoints DataJud](https://datajud-wiki.cnj.jus.br/api-publica/endpoints/)
- [Pesquisa por processo](https://datajud-wiki.cnj.jus.br/api-publica/exemplos/exemplo1/)
- [API de agregados IBGE](https://servicodados.ibge.gov.br/api/docs/agregados?versao=3)
- [BCB/SGS 433, janeiro/fevereiro de 2024](https://api.bcb.gov.br/dados/serie/bcdata.sgs.433/dados?formato=json&dataInicial=01/01/2024&dataFinal=29/02/2024)
- [IBGE/SIDRA 1737/63, janeiro/fevereiro de 2024](https://servicodados.ibge.gov.br/api/v3/agregados/1737/periodos/202401%7C202402/variaveis/63?localidades=N1%5Ball%5D)

## Limites e continuidade

Lei 14.905/2024, SELIC, taxa legal e pagamentos intermediários permanecem bloqueados
até homologação com fixtures oficiais e motor cronológico. Calendário não presume
feriados por tribunal ou equivalência completa CPC/CLT. Consulta OAB, paginação
integral das fontes, visualizador de peças, sincronização agendada, planos comerciais,
laudos públicos e assinatura com rotação permanecem pendentes. Admin e portal de
negócio continuam nas Sprints 4/5. Produção continua bloqueada.
