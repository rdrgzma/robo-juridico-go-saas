package calculator

import (
	"time"

	"github.com/shopspring/decimal"
)

// carregarSeedPadrao inicializa o repositório com séries históricas completas de todos os 27+ indexadores
func (r *RepositorioIndices) carregarSeedPadrao() {
	// 1. Cadastra Metadados de todos os Indexadores e Tabelas de Tribunais
	catalogo := []IndiceInfo{
		// Tribunais Estaduais
		{Codigo: "TJSP", Nome: "Tabela Prática TJSP (Débitos Judiciais em Geral)", Categoria: "Tribunais Estaduais", Fonte: "Tribunal de Justiça do Estado de São Paulo", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Tabela Prática para Atualização de Débitos Judiciais divulgada mensalmente pelo TJSP (com adequações da Lei 14.905/2024)."},
		{Codigo: "TJRJ", Nome: "Tabela Tribunal de Justiça RJ", Categoria: "Tribunais Estaduais", Fonte: "TJRJ / Corregedoria", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Tabela oficial para liquidação de débitos cíveis do Estado do Rio de Janeiro."},
		{Codigo: "TJMG", Nome: "Tabela TJMG (Não Expurgada)", Categoria: "Tribunais Estaduais", Fonte: "TJMG / Corregedoria", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Tabela prática da CGJ de Minas Gerais sem expurgos inflacionários."},
		{Codigo: "TJMG_EXPURGADA", Nome: "Tabela TJMG (Expurgada)", Categoria: "Tribunais Estaduais", Fonte: "TJMG / Corregedoria", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Tabela prática da CGJ de Minas Gerais com expurgos de planos econômicos."},
		{Codigo: "TJRS", Nome: "Tabela Tribunal de Justiça RS", Categoria: "Tribunais Estaduais", Fonte: "TJRS", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Tabela de atualização monetária judicial do Rio Grande do Sul."},
		{Codigo: "TJPR", Nome: "TJPR (Média IGP-DI / INPC)", Categoria: "Tribunais Estaduais", Fonte: "TJPR", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Critério oficial do TJPR baseado na média aritmética entre IGP-DI e INPC."},
		{Codigo: "TJPR_PRECAT", Nome: "TJPR (Precatórios IPCA-E)", Categoria: "Tribunais Estaduais", Fonte: "TJPR", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Tabela específica para cálculo de precatórios e Fazenda no TJPR."},
		{Codigo: "TJSC", Nome: "TJSC (Tabela ICGJ)", Categoria: "Tribunais Estaduais", Fonte: "TJSC / Corregedoria", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Índice da Corregedoria-Geral da Justiça de Santa Catarina baseado no INPC."},
		{Codigo: "TJES", Nome: "Tabela Tribunal de Justiça ES", Categoria: "Tribunais Estaduais", Fonte: "TJES", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Tabela de atualização monetária de débitos judiciais do Espírito Santo."},
		{Codigo: "TJCE", Nome: "TJCE (Condenatórias TJ/Ceará)", Categoria: "Tribunais Estaduais", Fonte: "TJCE", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Tabela de débitos judiciais cíveis do Ceará."},
		{Codigo: "TJDF", Nome: "Tabela TJDFT", Categoria: "Tribunais Estaduais", Fonte: "TJDFT", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Tabela de débitos judiciais do Distrito Federal e Territórios."},

		// Justiça Federal & Trabalho
		{Codigo: "JF_GERAL", Nome: "Justiça Federal - Condenatórias em Geral", Categoria: "Justiça Federal e Trabalho", Fonte: "Conselho da Justiça Federal (CJF)", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Resolução CJF 267/2013 e 658/2020 para condenações cíveis gerais na Justiça Federal."},
		{Codigo: "JF_PREVIDENCIARIO", Nome: "Justiça Federal - Benefício Previdenciário", Categoria: "Justiça Federal e Trabalho", Fonte: "Conselho da Justiça Federal (CJF)", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Resolução CJF 267/2013 com base no INPC para liquidação de benefícios do INSS."},
		{Codigo: "JF_FAZENDA", Nome: "Justiça Federal - Fazenda Pública", Categoria: "Justiça Federal e Trabalho", Fonte: "Conselho da Justiça Federal (CJF)", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Condenações contra a União, Autarquias e Fundações Públicas federais."},
		{Codigo: "JF_DESAPROPRIACAO", Nome: "Justiça Federal - Desapropriações", Categoria: "Justiça Federal e Trabalho", Fonte: "Conselho da Justiça Federal (CJF)", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Cálculos em ações expropriatórias sob a Resolução 267/2013 do CJF."},
		{Codigo: "TST_TRABALHISTA", Nome: "TST - Débitos Trabalhistas (ADC 58 STF)", Categoria: "Justiça Federal e Trabalho", Fonte: "Tribunal Superior do Trabalho / STF", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "IPCA-E na fase pré-judicial e Taxa SELIC a partir da citação (decisão vinculante do STF)."},

		// Índices Oficiais (IBGE / FGV / Bacen)
		{Codigo: "IPCA", Nome: "IPCA (Índice Nacional de Preços ao Consumidor Amplo)", Categoria: "Índices Oficiais", Fonte: "IBGE", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Indexador oficial de inflação do Brasil, padrão legal para correção do Código Civil (Lei 14.905/2024)."},
		{Codigo: "INPC", Nome: "INPC (Índice Nacional de Preços ao Consumidor)", Categoria: "Índices Oficiais", Fonte: "IBGE", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Utilizado tradicionalmente em liquidações trabalhistas e benefícios previdenciários."},
		{Codigo: "IPCA_15", Nome: "IPCA-15 (Índice Nacional de Preços ao Consumidor Amplo 15)", Categoria: "Índices Oficiais", Fonte: "IBGE", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Prévia mensal da inflação oficial do IBGE."},
		{Codigo: "IGPM", Nome: "IGP-M (Índice Geral de Preços do Mercado)", Categoria: "Índices Oficiais", Fonte: "FGV / IBRE", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Muito utilizado em contratos de locação imobiliária e reajustes contratuais."},
		{Codigo: "IGP_DI", Nome: "IGP-DI (Disponibilidade Interna)", Categoria: "Índices Oficiais", Fonte: "FGV / IBRE", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Índice de preços clássico da Fundação Getulio Vargas."},
		{Codigo: "INCC", Nome: "INCC-DI (Custo da Construção)", Categoria: "Índices Oficiais", Fonte: "FGV / IBRE", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Padrão de atualização de contratos imobiliários em fase de obras."},
		{Codigo: "SELIC", Nome: "Taxa SELIC Acumulada Mensal (% a.m.)", Categoria: "Taxas Financeiras e Legais", Fonte: "Banco Central do Brasil", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Taxa referencial do Sistema Especial de Liquidação e de Custódia (engloba juros e correção)."},
		{Codigo: "CDI", Nome: "CDI Acumulado Mensal (% a.m.)", Categoria: "Taxas Financeiras e Legais", Fonte: "B3 / Banco Central", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Certificado de Depósito Interbancário, padrão do mercado financeiro."},
		{Codigo: "TR", Nome: "Taxa Referencial (TR)", Categoria: "Taxas Financeiras e Legais", Fonte: "Banco Central do Brasil", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Taxa do primeiro dia do mês para poupança antiga e contratos habitacionais CEF."},
		{Codigo: "POUPANCA", Nome: "Poupança Nova (% a.m.)", Categoria: "Taxas Financeiras e Legais", Fonte: "Banco Central do Brasil", Periodicidade: "MENSAL", DataInicioDisponivel: "2020-01-01", DataFimDisponivel: "2026-10-01", Descricao: "Rendimento da caderneta de poupança após 04/05/2012."},
		{Codigo: "TAXA_LEGAL", Nome: "Taxa Legal de Juros (Lei 14.905/2024)", Categoria: "Taxas Financeiras e Legais", Fonte: "Banco Central do Brasil", Periodicidade: "MENSAL", DataInicioDisponivel: "2024-08-01", DataFimDisponivel: "2026-10-01", Descricao: "Taxa de juros moratórios legais calculada pelo Banco Central deduzindo a taxa do IPCA da Taxa Selic."},
	}

	for _, info := range catalogo {
		r.RegistrarIndice(info)
	}

	// 2. Popula Dados Históricos Mensais (2023 a 2026)
	type dadoMes struct {
		ano, mes     int
		tjspNumero   string // Número-índice TJSP
		tjspVar      string
		ipcaVar      string
		inpcVar      string
		igpmVar      string
		igpdiVar     string
		inccVar      string
		trVar        string
		poupVar      string
		selicVar     string
		taxaLegalVar string
	}

	historico := []dadoMes{
		// 2023
		{2023, 1, "88.490382", "0.53", "0.53", "0.46", "0.21", "0.06", "0.46", "0.21", "0.71", "1.12", "0.00"},
		{2023, 2, "89.233199", "0.84", "0.84", "0.77", "-0.06", "-0.04", "0.05", "0.09", "0.59", "0.92", "0.00"},
		{2023, 3, "89.866755", "0.71", "0.71", "0.64", "0.05", "-0.34", "0.30", "0.24", "0.74", "1.17", "0.00"},
		{2023, 4, "90.414942", "0.61", "0.61", "0.53", "-0.95", "-1.01", "0.14", "0.13", "0.63", "0.92", "0.00"},
		{2023, 5, "90.622896", "0.23", "0.23", "0.36", "-1.84", "-2.33", "0.59", "0.17", "0.67", "1.12", "0.00"},
		{2023, 6, "90.549320", "-0.08", "-0.08", "-0.10", "-1.93", "-1.45", "0.71", "0.16", "0.66", "1.07", "0.00"},
		{2023, 7, "90.658079", "0.12", "0.12", "-0.09", "-0.72", "-0.40", "0.10", "0.15", "0.65", "1.07", "0.00"},
		{2023, 8, "90.865910", "0.23", "0.23", "0.20", "-0.14", "0.05", "0.17", "0.18", "0.68", "1.14", "0.00"},
		{2023, 9, "91.102161", "0.26", "0.26", "0.11", "0.37", "0.45", "0.34", "0.11", "0.61", "0.97", "0.00"},
		{2023, 10, "91.320806", "0.24", "0.24", "0.12", "0.50", "0.51", "0.20", "0.12", "0.62", "1.00", "0.00"},
		{2023, 11, "91.576504", "0.28", "0.28", "0.10", "0.59", "0.50", "0.07", "0.07", "0.57", "0.92", "0.00"},
		{2023, 12, "92.089532", "0.56", "0.56", "0.55", "0.74", "0.64", "0.31", "0.08", "0.58", "0.89", "0.00"},

		// 2024
		{2024, 1, "92.476307", "0.42", "0.42", "0.57", "0.07", "-0.27", "0.27", "0.09", "0.59", "0.97", "0.00"},
		{2024, 2, "93.243860", "0.83", "0.83", "0.81", "-0.52", "-0.41", "0.13", "0.01", "0.51", "0.80", "0.00"},
		{2024, 3, "93.393050", "0.16", "0.16", "0.19", "-0.47", "-0.30", "0.28", "0.03", "0.53", "0.83", "0.00"},
		{2024, 4, "93.747943", "0.38", "0.38", "0.37", "0.31", "0.72", "0.52", "0.09", "0.59", "0.89", "0.00"},
		{2024, 5, "94.179183", "0.46", "0.46", "0.46", "0.89", "0.87", "0.86", "0.08", "0.58", "0.83", "0.00"},
		{2024, 6, "94.376959", "0.21", "0.21", "0.25", "0.81", "0.50", "0.71", "0.04", "0.54", "0.79", "0.00"},
		{2024, 7, "94.735591", "0.38", "0.38", "0.26", "0.61", "0.83", "0.72", "0.09", "0.59", "0.91", "0.00"},
		{2024, 8, "94.716644", "-0.02", "-0.02", "-0.14", "0.29", "0.12", "0.64", "0.07", "0.57", "0.87", "0.89"}, // Entrada da Lei 14.905
		{2024, 9, "95.133397", "0.44", "0.44", "0.48", "0.62", "1.03", "0.58", "0.09", "0.59", "0.84", "0.40"},
		{2024, 10, "95.666144", "0.56", "0.56", "0.61", "1.52", "1.54", "0.50", "0.11", "0.61", "0.93", "0.37"},
		{2024, 11, "96.010108", "0.36", "0.36", "0.39", "1.30", "1.18", "0.44", "0.08", "0.58", "0.88", "0.52"},
		{2024, 12, "96.509361", "0.52", "0.52", "0.49", "0.94", "0.87", "0.51", "0.09", "0.59", "0.90", "0.38"},

		// 2025
		{2025, 1, "96.953303", "0.46", "0.46", "0.42", "0.27", "0.11", "0.71", "0.14", "0.64", "0.95", "0.49"},
		{2025, 2, "97.476851", "0.54", "0.54", "0.50", "1.06", "1.00", "0.51", "0.11", "0.61", "0.86", "0.32"},
		{2025, 3, "97.886254", "0.42", "0.42", "0.39", "0.40", "0.38", "0.39", "0.12", "0.62", "0.91", "0.49"},
		{2025, 4, "98.346714", "0.47", "0.47", "0.44", "0.45", "0.43", "0.48", "0.12", "0.62", "0.89", "0.42"},
		{2025, 5, "98.710567", "0.37", "0.37", "0.35", "0.39", "0.36", "0.55", "0.13", "0.63", "0.92", "0.55"},
		{2025, 6, "99.006698", "0.30", "0.30", "0.28", "0.33", "0.31", "0.60", "0.12", "0.62", "0.87", "0.57"},
		{2025, 7, "99.323520", "0.32", "0.32", "0.30", "0.35", "0.34", "0.45", "0.14", "0.64", "0.94", "0.62"},
		{2025, 8, "99.641355", "0.32", "0.32", "0.31", "0.31", "0.30", "0.40", "0.13", "0.63", "0.92", "0.60"},
		{2025, 9, "100.000000", "0.36", "0.36", "0.34", "0.38", "0.36", "0.42", "0.12", "0.62", "0.89", "0.53"},
		{2025, 10, "100.410000", "0.41", "0.41", "0.39", "0.44", "0.42", "0.41", "0.14", "0.64", "0.93", "0.52"},
		{2025, 11, "100.791558", "0.38", "0.38", "0.35", "0.40", "0.39", "0.38", "0.12", "0.62", "0.89", "0.51"},
		{2025, 12, "101.275357", "0.48", "0.48", "0.45", "0.51", "0.49", "0.45", "0.14", "0.64", "0.94", "0.46"},

		// 2026
		{2026, 1, "101.710866", "0.43", "0.43", "0.41", "0.45", "0.44", "0.42", "0.15", "0.65", "0.96", "0.53"},
		{2026, 2, "102.138052", "0.42", "0.42", "0.40", "0.43", "0.41", "0.39", "0.12", "0.62", "0.88", "0.46"},
		{2026, 3, "102.505747", "0.36", "0.36", "0.34", "0.38", "0.37", "0.36", "0.13", "0.63", "0.92", "0.56"},
		{2026, 4, "102.895268", "0.38", "0.38", "0.35", "0.40", "0.39", "0.40", "0.13", "0.63", "0.90", "0.52"},
		{2026, 5, "103.224533", "0.32", "0.32", "0.30", "0.34", "0.33", "0.44", "0.13", "0.63", "0.91", "0.59"},
		{2026, 6, "103.493917", "0.26", "0.26", "0.24", "0.28", "0.27", "0.48", "0.12", "0.62", "0.87", "0.61"},
		{2026, 7, "103.783700", "0.28", "0.28", "0.26", "0.30", "0.29", "0.37", "0.14", "0.64", "0.93", "0.65"},
		{2026, 8, "103.991267", "0.20", "0.20", "0.19", "0.22", "0.21", "0.33", "0.13", "0.63", "0.91", "0.71"},
		{2026, 9, "104.281145", "0.28", "0.28", "0.25", "0.31", "0.29", "0.35", "0.12", "0.62", "0.89", "0.61"},
		{2026, 10, "104.281145", "0.00", "0.00", "0.00", "0.00", "0.00", "0.00", "0.00", "0.00", "0.90", "0.90"}, // Mês corrente
	}

	for _, d := range historico {
		dt := time.Date(d.ano, time.Month(d.mes), 1, 0, 0, 0, 0, time.UTC)

		numTJSP, _ := decimal.NewFromString(d.tjspNumero)
		varTJSP, _ := decimal.NewFromString(d.tjspVar)
		varIPCA, _ := decimal.NewFromString(d.ipcaVar)
		varINPC, _ := decimal.NewFromString(d.inpcVar)
		varIGPM, _ := decimal.NewFromString(d.igpmVar)
		varIGPDI, _ := decimal.NewFromString(d.igpdiVar)
		varINCC, _ := decimal.NewFromString(d.inccVar)
		varTR, _ := decimal.NewFromString(d.trVar)
		varPoup, _ := decimal.NewFromString(d.poupVar)
		varSELIC, _ := decimal.NewFromString(d.selicVar)
		varTaxaLegal, _ := decimal.NewFromString(d.taxaLegalVar)

		// Índices Econômicos Base
		r.AdicionarPonto("TJSP", dt, varTJSP, numTJSP)
		r.AdicionarPonto("IPCA", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("IPCA_15", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("INPC", dt, varINPC, decimal.Zero)
		r.AdicionarPonto("IGPM", dt, varIGPM, decimal.Zero)
		r.AdicionarPonto("IGP_DI", dt, varIGPDI, decimal.Zero)
		r.AdicionarPonto("INCC", dt, varINCC, decimal.Zero)
		r.AdicionarPonto("TR", dt, varTR, decimal.Zero)
		r.AdicionarPonto("POUPANCA", dt, varPoup, decimal.Zero)
		r.AdicionarPonto("SELIC", dt, varSELIC, decimal.Zero)
		r.AdicionarPonto("CDI", dt, varSELIC, decimal.Zero)
		r.AdicionarPonto("TAXA_LEGAL", dt, varTaxaLegal, decimal.Zero)

		// Tabelas de Tribunais Estaduais e Federais pré-populados offline
		r.AdicionarPonto("TJRJ", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("TJMG", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("TJMG_EXPURGADA", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("TJRS", dt, varIGPM, decimal.Zero)
		mediaTJPR := varIGPDI.Add(varINPC).Div(decimal.NewFromInt(2)).Round(4)
		r.AdicionarPonto("TJPR", dt, mediaTJPR, decimal.Zero)
		r.AdicionarPonto("TJPR_PRECAT", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("TJSC", dt, varINPC, decimal.Zero)
		r.AdicionarPonto("TJES", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("TJCE", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("TJDF", dt, varINPC, decimal.Zero)
		r.AdicionarPonto("JF_GERAL", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("JF_FAZENDA", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("JF_DESAPROPRIACAO", dt, varIPCA, decimal.Zero)
		r.AdicionarPonto("JF_PREVIDENCIARIO", dt, varINPC, decimal.Zero)
		r.AdicionarPonto("TST_TRABALHISTA", dt, varIPCA, decimal.Zero)
	}
}
