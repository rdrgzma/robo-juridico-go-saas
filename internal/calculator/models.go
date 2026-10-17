package calculator

import (
	"time"

	"github.com/shopspring/decimal"
)

// TipoParcela define a natureza de uma parcela na liquidação
type TipoParcela string

const (
	ParcelaDebito           TipoParcela = "DEBITO"
	ParcelaCusta            TipoParcela = "CUSTA_PROCESSUAL"
	ParcelaDespesa          TipoParcela = "DESPESA_PROCESSUAL"
	ParcelaPagamentoParcial TipoParcela = "PAGAMENTO_PARCIAL"
)

// TipoJuros define a regra legal ou convencional aplicável
type TipoJuros string

const (
	JurosConvencionalMensal TipoJuros = "CONVENCIONAL_MENSAL" // Ex: 1.0% a.m.
	JurosTaxaLegal          TipoJuros = "TAXA_LEGAL"          // Lei 14.905/2024: SELIC menos IPCA
	JurosLei14905           TipoJuros = "LEI_14905"           // Alias compatível com UI pública para TAXA_LEGAL
	JurosSelic              TipoJuros = "SELIC"               // Selic pura (juros + correção)
	JurosNenhum             TipoJuros = "SEM_JUROS"           // Apenas correção
)

// ModoCapitalizacao define a forma de acumulação dos juros
type ModoCapitalizacao string

const (
	CapitalizacaoSimples  ModoCapitalizacao = "SIMPLES"
	CapitalizacaoComposta ModoCapitalizacao = "COMPOSTO"
)

// IndiceInfo representa um indexador econômico cadastrado
type IndiceInfo struct {
	Codigo               string `json:"codigo"`
	Nome                 string `json:"nome"`
	Categoria            string `json:"categoria,omitempty"`
	Fonte                string `json:"fonte"`
	Periodicidade        string `json:"periodicidade"`
	DataInicioDisponivel string `json:"data_inicio_disponivel"`
	DataFimDisponivel    string `json:"data_fim_disponivel"`
	Descricao            string `json:"descricao"`
}

// PontoSerie representa um ponto na série histórica do índice
type PontoSerie struct {
	DataReferencia      time.Time       `json:"data_referencia"`
	DataReferenciaTexto string          `json:"data_referencia_texto"` // Ex: "2024-01"
	ValorPercentual     decimal.Decimal `json:"valor_percentual"`      // Variação percentual do mês
	FatorMultiplicador  decimal.Decimal `json:"fator_multiplicador"`   // 1 + (taxa/100)
	NumeroIndice        decimal.Decimal `json:"numero_indice"`         // Número-índice acumulado
}

// ParcelaItem representa uma entrada individual de valor a ser liquidada
type ParcelaItem struct {
	NumeroItem      int             `json:"numero_item"`
	Descricao       string          `json:"descricao"`
	Tipo            TipoParcela     `json:"tipo"`
	DataVencimento  string          `json:"data_vencimento"`             // Formato "AAAA-MM-DD" ou "DD/MM/AAAA"
	Data            string          `json:"data,omitempty"`              // Alias de compatibilidade para importadores frontend
	DataInicioJuros string          `json:"data_inicio_juros,omitempty"` // Se vazia, usa DataVencimento
	ValorOriginal   decimal.Decimal `json:"valor_original"`
}

// JurosMoraConfig define os parâmetros de juros moratórios
type JurosMoraConfig struct {
	Tipo              TipoJuros         `json:"tipo"`                        // CONVENCIONAL_MENSAL, TAXA_LEGAL, LEI_14905, SELIC, SEM_JUROS
	PercentualMensal  decimal.Decimal   `json:"percentual_mensal"`           // Ex: 1.0 para 1% a.m.
	Modo              ModoCapitalizacao `json:"modo"`                        // SIMPLES ou COMPOSTO
	DataInicioGeral   string            `json:"data_inicio_geral,omitempty"` // Se preenchido, sobrepõe a data de cada parcela (ex: data da citação)
	AplicarProRataDie bool              `json:"aplicar_prorata_die"`
}

// JurosCompensatoriosConfig define juros compensatórios / remuneratórios (opcional)
type JurosCompensatoriosConfig struct {
	Ativo            bool              `json:"ativo"`
	PercentualMensal decimal.Decimal   `json:"percentual_mensal"`
	TaxaMensal       decimal.Decimal   `json:"taxa_mensal,omitempty"`
	Modo             ModoCapitalizacao `json:"modo"`
	DataInicio       string            `json:"data_inicio,omitempty"`
	DataFim          string            `json:"data_fim,omitempty"`
}

// FaseExecucaoConfig reúne encargos do Art. 523 CPC e honorários sucumbenciais
type FaseExecucaoConfig struct {
	AplicarArt523Multa                bool            `json:"aplicar_art_523_multa"`               // Multa de 10%
	AplicarArt523Honorarios           bool            `json:"aplicar_art_523_honorarios"`          // Honorários executivos de 10%
	HonorariosSucumbenciaisPercentual decimal.Decimal `json:"honorarios_sucumbenciais_percentual"` // Ex: 10% a 20%
	HonorariosSucumbenciaisValorFixo  decimal.Decimal `json:"honorarios_sucumbenciais_valor_fixo"` // Valor fixo em R$
	MultaMoratoriaPercentual          decimal.Decimal `json:"multa_moratoria_percentual"`          // Ex: 2% a 10% contratual/moratória
}

// CalculoJudicialRequest é o payload completo de requisição de cálculo
type CalculoJudicialRequest struct {
	NumeroProcesso         string                    `json:"numero_processo,omitempty"`
	DataAtualizacao        string                    `json:"data_atualizacao"`         // Data ad quem ("AAAA-MM-DD" ou "DD/MM/AAAA")
	IndexadorCodigo        string                    `json:"indexador_codigo"`         // Ex: "TJSP", "IPCA", "INPC", "IGPM"
	AplicarDeflacao        bool                      `json:"aplicar_deflacao"`         // Se false, variações negativas viram 0
	AplicarProRataCorrecao bool                      `json:"aplicar_prorata_correcao"` // Se true, interpola pro rata die na correção monetária
	ImputarArt354CC        bool                      `json:"imputar_art_354_cc"`       // Se true, pagamentos parciais abatem primeiro juros (Art. 354 CC)
	JurosMora              JurosMoraConfig           `json:"juros_mora"`
	JurosCompensatorios    JurosCompensatoriosConfig `json:"juros_compensatorios,omitempty"`
	FaseExecucao           FaseExecucaoConfig        `json:"fase_execucao"`
	Parcelas               []ParcelaItem             `json:"parcelas"`
}

// ItemLiquidado representa a memória analítica de cálculo de uma parcela
type ItemLiquidado struct {
	NumeroItem               int             `json:"numero_item"`
	Descricao                string          `json:"descricao"`
	Tipo                     TipoParcela     `json:"tipo"`
	DataVencimento           string          `json:"data_vencimento"`
	DataVencimentoISO        string          `json:"data_vencimento_iso,omitempty"`
	DataInicioJuros          string          `json:"data_inicio_juros"`
	ValorOriginal            decimal.Decimal `json:"valor_original"`
	FatorCorrecao            decimal.Decimal `json:"fator_correcao"`
	ValorCorrigido           decimal.Decimal `json:"valor_corrigido"`
	TaxaJurosAcumulada       decimal.Decimal `json:"taxa_juros_acumulada_percentual"` // Ex: 45.32%
	ValorJurosMora           decimal.Decimal `json:"valor_juros_mora"`
	ValorJuros               decimal.Decimal `json:"valor_juros"` // Alias compatível com Workbook / UI Pública
	ValorJurosCompensatorios decimal.Decimal `json:"valor_juros_compensatorios"`
	ValorMultaMoratoria      decimal.Decimal `json:"valor_multa_moratoria"`
	TotalItem                decimal.Decimal `json:"total_item"`
	Subtotal                 decimal.Decimal `json:"subtotal"` // Alias compatível com Workbook / UI Pública
	Observacoes              string          `json:"observacoes,omitempty"`
}

// ResumoCalculo consolida todos os totais e encargos finais da liquidação
type ResumoCalculo struct {
	DataAtualizacao          string          `json:"data_atualizacao"`
	IndexadorCodigo          string          `json:"indexador"`
	IndexadorNome            string          `json:"indexador_nome"`
	TotalPrincipalOriginal   decimal.Decimal `json:"total_principal_original"`
	TotalPrincipalCorrigido  decimal.Decimal `json:"total_principal_corrigido"`
	TotalCorrecaoMonetaria   decimal.Decimal `json:"total_correcao_monetaria"`
	TotalJurosMora           decimal.Decimal `json:"total_juros_mora"`
	TotalJurosCompensatorios decimal.Decimal `json:"total_juros_compensatorios"`
	TotalJuros               decimal.Decimal `json:"total_juros"` // Soma de juros moratórios + compensatórios
	TotalMultaMoratoria      decimal.Decimal `json:"total_multa_moratoria"`
	TotalCustasEDespesas     decimal.Decimal `json:"total_custas_e_despesas"`
	TotalAbatimentos         decimal.Decimal `json:"total_abatimentos"`
	ImputadoJurosArt354      decimal.Decimal `json:"imputado_juros_art354"`
	ImputadoPrincipalArt354  decimal.Decimal `json:"imputado_principal_art354"`
	SubtotalCondenacao       decimal.Decimal `json:"subtotal_condenacao"`
	MultaArt523CPC           decimal.Decimal `json:"multa_art_523_cpc"`
	HonorariosArt523CPC      decimal.Decimal `json:"honorarios_art_523_cpc"`
	HonorariosSucumbenciais  decimal.Decimal `json:"honorarios_sucumbenciais"`
	TotalMultas              decimal.Decimal `json:"total_multas"`     // Soma de Multa Moratória + Multa Art. 523
	TotalHonorarios          decimal.Decimal `json:"total_honorarios"` // Soma de Honorários Art. 523 + Sucumbenciais
	TotalGeralDevido         decimal.Decimal `json:"total_geral_devido"`
	TotalGeral               decimal.Decimal `json:"total_geral"` // Alias compatível com Workbook / UI Pública
}

// CalculoJudicialResponse é o resultado estruturado retornado pela API
type CalculoJudicialResponse struct {
	OK                 bool            `json:"ok"`
	Sucesso            bool            `json:"sucesso"`
	Mensagem           string          `json:"mensagem,omitempty"`
	Resumo             ResumoCalculo   `json:"resumo"`
	ItensLiquidados    []ItemLiquidado `json:"itens_liquidados"`
	Itens              []ItemLiquidado `json:"itens"` // Alias compatível com Workbook / UI Pública
	NotasMetodologicas []string        `json:"notas_metodologicas"`
	NotasLegais        []string        `json:"notas_legais"` // Alias compatível com Workbook / UI Pública
}

// SistemaAmortizacao define o método revisional de financiamento bancário
type SistemaAmortizacao string

const (
	SistemaPrice SistemaAmortizacao = "PRICE"
	SistemaSAC   SistemaAmortizacao = "SAC"
	SistemaGauss SistemaAmortizacao = "GAUSS"
)

// CalculoFinanciamentoRequest representa o payload para revisão contratual bancária (Price, SAC, Gauss)
type CalculoFinanciamentoRequest struct {
	SistemaAmortizacao   SistemaAmortizacao `json:"sistema_amortizacao"`              // "PRICE", "SAC" ou "GAUSS"
	ValorFinanciado      decimal.Decimal    `json:"valor_financiado"`                 // Ex: 50000.00
	NumeroParcelas       int                `json:"numero_parcelas"`                  // Ex: 48
	QuantidadeParcelas   int                `json:"quantidade_parcelas,omitempty"`    // Alias
	TaxaContratadaMensal decimal.Decimal    `json:"taxa_contratada_mensal"`           // Ex: 2.85 (% a.m.)
	TaxaMensalContratada decimal.Decimal    `json:"taxa_mensal_contratada,omitempty"` // Alias
	TaxaMercadoMensal    decimal.Decimal    `json:"taxa_mercado_mensal"`              // Ex: 1.45 (% a.m. - Taxa Média Bacen)
	TaxaMensalMercado    decimal.Decimal    `json:"taxa_mensal_mercado,omitempty"`    // Alias
	RestituicaoEmDobro   bool               `json:"restituicao_em_dobro"`             // Art. 42, parágrafo único do CDC
	RepeticaoEmDobro     bool               `json:"repeticao_em_dobro,omitempty"`     // Alias
}

// ParcelaFinanciamentoItem detalha a evolução mês a mês do contrato vs revisão
type ParcelaFinanciamentoItem struct {
	NumeroParcela       int             `json:"numero_parcela"`
	PrestacaoContratada decimal.Decimal `json:"prestacao_contratada"`
	PrestacaoRevisada   decimal.Decimal `json:"prestacao_revisada"`
	DiferencaParcela    decimal.Decimal `json:"diferenca_parcela"`
	JurosRevisado       decimal.Decimal `json:"juros_revisado"`
	AmortizacaoRevisada decimal.Decimal `json:"amortizacao_revisada"`
	SaldoDevedor        decimal.Decimal `json:"saldo_devedor"`
}

// CalculoFinanciamentoResponse retorna o laudo revisional bancário completo
type CalculoFinanciamentoResponse struct {
	Sucesso                  bool                       `json:"sucesso"`
	SistemaAmortizacao       SistemaAmortizacao         `json:"sistema_amortizacao"`
	ValorFinanciado          decimal.Decimal            `json:"valor_financiado"`
	NumeroParcelas           int                        `json:"numero_parcelas"`
	TaxaContratadaMensal     decimal.Decimal            `json:"taxa_contratada_mensal"`
	TaxaMercadoMensal        decimal.Decimal            `json:"taxa_mercado_mensal"`
	PrestacaoContratada      decimal.Decimal            `json:"prestacao_contratada"`
	PrestacaoRevisadaInicial decimal.Decimal            `json:"prestacao_revisada_inicial"`
	TotalPagoContratado      decimal.Decimal            `json:"total_pago_contratado"`
	TotalDevidoRevisado      decimal.Decimal            `json:"total_devido_revisado"`
	DiferencaTotalSimples    decimal.Decimal            `json:"diferenca_total_simples"`
	IndebitoCobrado          decimal.Decimal            `json:"indebito_cobrado"`
	TabelaEvolucao           []ParcelaFinanciamentoItem `json:"tabela_evolucao"`
	NotasMetodologicas       []string                   `json:"notas_metodologicas"`
}
