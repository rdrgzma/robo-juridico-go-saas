# Proveniência e autorização de reaproveitamento

Em 17/10/2026, o usuário declarou ser titular dos fontes e autorizou expressamente
copiá-los e adaptá-los neste projeto. Essa autorização resolve a pendência de uso
interno registrada na primeira auditoria; não presume uma licença MIT global.
Nenhuma nova licença pública de distribuição foi escolhida pelo agente.

Origem: rdrgzma/robo-juridico-api, commit
12685bbe7a5be048fcaf01332f67503a09b0bee8, diretório
4-api-robo-juridico-php-go/api-juridica-go/internal/calculator/.

Arquivos inicialmente reaproveitados: models.go, engine.go, correction.go,
interest.go, penalties.go, repository.go, financing.go, importer.go, exporter.go,
engine_test.go, importer_test.go e seed.go. Os dois testes receberam nomes
*_legacy_test.go; seed.go foi movido para seed_legacy_test.go, exclusivamente para
caracterização em testes. Avisos/comentários originais foram preservados quando
aplicáveis; adaptações estão no histórico Git e na auditoria.

O financiamento foi consolidado sobre Finance, removendo a cópia de fórmulas e
float64. O parser XLSX foi substituído por implementação limitada que preserva
células esparsas e inline strings. O engine recebeu validações e rejeições explícitas.

Não foram copiados .env, tokens, bancos, dados pessoais, executáveis ou PHP.
O repositório original foi acessado somente por leitura e nunca recebeu commit/push.

Na Sprint 3, a paleta, organização do menu e alternância de tema tomam como
referência os arquivos 5-app-juridico-desktop/ui/static/index.html e style.css no
mesmo commit auditado. Foram adaptados para templates Go, permissões e histórico
PostgreSQL. Clientes externos são novos, orientados por contratos oficiais e pela
inspeção dos clientes DataJud/Comunica do legado; não copiam chaves, proxies ou dados.
