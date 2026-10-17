package calculator

import (
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

// EngineCalculo orquestra a liquidação judicial completa de débitos e créditos
type EngineCalculo struct {
	repo *RepositorioIndices
}

func NovoEngineCalculo(repo *RepositorioIndices) *EngineCalculo {
	if repo == nil {
		repo = NovoRepositorioIndices()
	}
	return &EngineCalculo{repo: repo}
}

func (e *EngineCalculo) Repositorio() *RepositorioIndices {
	return e.repo
}

// Executar realiza a liquidação analítica e sintética das parcelas informadas
func (e *EngineCalculo) Executar(req CalculoJudicialRequest) (*CalculoJudicialResponse, error) {
	if err := validarCalculo(req); err != nil {
		return nil, err
	}
	req.Parcelas = append([]ParcelaItem(nil), req.Parcelas...)
	sort.SliceStable(req.Parcelas, func(i, j int) bool {
		return req.Parcelas[i].Tipo != ParcelaPagamentoParcial && req.Parcelas[j].Tipo == ParcelaPagamentoParcial
	})
	if len(req.Parcelas) == 0 {
		return nil, fmt.Errorf("nenhuma parcela informada para liquidacao")
	}

	dataAtualizacao, err := ParseDataFlexivel(req.DataAtualizacao)
	if err != nil {
		return nil, fmt.Errorf("data de atualizacao invalida ('%s'): %w", req.DataAtualizacao, err)
	}

	indiceInfo, err := e.repo.ObterIndice(req.IndexadorCodigo)
	if err != nil {
		return nil, err
	}

	// Data inicial de juros global se especificada
	var dataInicioJurosGlobal *time.Time
	if req.JurosMora.DataInicioGeral != "" {
		dt, err := ParseDataFlexivel(req.JurosMora.DataInicioGeral)
		if err == nil {
			dataInicioJurosGlobal = &dt
		}
	}

	itensLiquidados := make([]ItemLiquidado, 0, len(req.Parcelas))

	totalPrincipalOriginal := decimal.Zero
	totalPrincipalCorrigido := decimal.Zero
	totalJurosMora := decimal.Zero
	totalJurosCompensatorios := decimal.Zero
	totalMultaMoratoria := decimal.Zero
	totalCustasEDespesas := decimal.Zero
	totalAbatimentos := decimal.Zero
	totalCorrecao := decimal.Zero
	totalImputadoJuros, totalImputadoPrincipal := decimal.Zero, decimal.Zero

	for idx, p := range req.Parcelas {
		strDataVencto := p.DataVencimento
		if strDataVencto == "" {
			strDataVencto = p.Data
		}
		dtVencto, err := ParseDataFlexivel(strDataVencto)
		if err != nil {
			return nil, fmt.Errorf("item %d: data de vencimento invalida ('%s'): %w", idx+1, strDataVencto, err)
		}

		dtJuros := dtVencto
		if dataInicioJurosGlobal != nil {
			dtJuros = *dataInicioJurosGlobal
		} else if p.DataInicioJuros != "" {
			dtJ, err := ParseDataFlexivel(p.DataInicioJuros)
			if err == nil {
				dtJuros = dtJ
			}
		}

		// 1. Correção Monetária (com suporte opcional a pro rata die)
		fatorCorrecao, err := e.repo.ObterFatorAcumuladoComOpcoes(
			req.IndexadorCodigo,
			dtVencto,
			dataAtualizacao,
			req.AplicarDeflacao,
			req.AplicarProRataCorrecao,
		)
		if err != nil {
			return nil, fmt.Errorf("item %d: correção indisponível: %w", idx+1, err)
		}

		valorOriginal := p.ValorOriginal
		valorCorrigido := valorOriginal.Mul(fatorCorrecao).Round(2)

		taxaJurosAcumulada := decimal.Zero
		valorJuros := decimal.Zero
		valorJurosCompItem := decimal.Zero
		valorMultaItem := decimal.Zero
		obs := ""

		switch p.Tipo {
		case ParcelaDebito, "":
			// Débitos sofrem juros moratórios, compensatórios (se ativos) e multa
			taxaJuros, vJuros, obsJuros, errJuros := CalcularJurosMoratorios(
				e.repo,
				req.JurosMora,
				dtJuros,
				dataAtualizacao,
				valorCorrigido,
			)
			if errJuros != nil {
				return nil, errJuros
			}
			taxaJurosAcumulada = taxaJuros
			valorJuros = vJuros
			obs = obsJuros

			if req.JurosCompensatorios.Ativo {
				_, vComp, obsComp := CalcularJurosCompensatorios(
					req.JurosCompensatorios,
					dtVencto,
					dataAtualizacao,
					valorCorrigido,
				)
				valorJurosCompItem = vComp
				if obsComp != "" {
					if obs != "" {
						obs += " | " + obsComp
					} else {
						obs = obsComp
					}
				}
			}

			if !req.FaseExecucao.MultaMoratoriaPercentual.IsZero() {
				valorMultaItem = valorCorrigido.Mul(req.FaseExecucao.MultaMoratoriaPercentual).Div(decimal.NewFromInt(100)).Round(2)
			}

			totalPrincipalOriginal = totalPrincipalOriginal.Add(valorOriginal)
			totalCorrecao = totalCorrecao.Add(valorCorrigido.Sub(valorOriginal))
			totalPrincipalCorrigido = totalPrincipalCorrigido.Add(valorCorrigido)
			totalJurosMora = totalJurosMora.Add(valorJuros)
			totalJurosCompensatorios = totalJurosCompensatorios.Add(valorJurosCompItem)
			totalMultaMoratoria = totalMultaMoratoria.Add(valorMultaItem)

		case ParcelaCusta, ParcelaDespesa:
			// Custas sofrem apenas correção monetária
			obs = "Atualização monetária sem incidência de juros ou multa"
			totalCustasEDespesas = totalCustasEDespesas.Add(valorCorrigido)

		case ParcelaPagamentoParcial:
			// Abatimento/Pagamento Parcial (subtrai do saldo)
			if valorOriginal.IsPositive() {
				valorOriginal = valorOriginal.Neg()
				valorCorrigido = valorCorrigido.Neg()
			}
			valAbatimentoAbs := valorCorrigido.Abs()
			if req.ImputarArt354CC {
				if valAbatimentoAbs.GreaterThan(totalJurosMora.Add(totalPrincipalCorrigido)) {
					return nil, fmt.Errorf("pagamento excede juros moratórios e principal disponíveis para imputação")
				}
				// Art. 354 CC: imputa primeiro nos juros vencidos/acumulados e o excedente no capital
				abatJuros := decimal.Min(valAbatimentoAbs, totalJurosMora)
				abatPrincipal := valAbatimentoAbs.Sub(abatJuros)
				totalImputadoJuros = totalImputadoJuros.Add(abatJuros)
				totalImputadoPrincipal = totalImputadoPrincipal.Add(abatPrincipal)
				totalJurosMora = totalJurosMora.Sub(abatJuros)
				totalPrincipalCorrigido = totalPrincipalCorrigido.Sub(abatPrincipal)
				obs = fmt.Sprintf("Imputação Art. 354 CC: R$ %s abatidos dos juros e R$ %s do capital principal", abatJuros.StringFixed(2), abatPrincipal.StringFixed(2))
			} else {
				obs = "Pagamento parcial deduzido da conta geral"
				totalAbatimentos = totalAbatimentos.Add(valAbatimentoAbs)
			}
		}

		totalItem := valorCorrigido.Add(valorJuros).Add(valorJurosCompItem).Add(valorMultaItem)

		numItem := p.NumeroItem
		if numItem == 0 {
			numItem = idx + 1
		}

		tipoParcela := p.Tipo
		if tipoParcela == "" {
			tipoParcela = ParcelaDebito
		}

		itensLiquidados = append(itensLiquidados, ItemLiquidado{
			NumeroItem:               numItem,
			Descricao:                p.Descricao,
			Tipo:                     tipoParcela,
			DataVencimento:           dtVencto.Format("02/01/2006"),
			DataInicioJuros:          dtJuros.Format("02/01/2006"),
			ValorOriginal:            valorOriginal,
			FatorCorrecao:            fatorCorrecao,
			ValorCorrigido:           valorCorrigido,
			TaxaJurosAcumulada:       taxaJurosAcumulada,
			ValorJurosMora:           valorJuros,
			ValorJuros:               valorJuros.Add(valorJurosCompItem),
			ValorJurosCompensatorios: valorJurosCompItem,
			ValorMultaMoratoria:      valorMultaItem,
			TotalItem:                totalItem,
			Subtotal:                 totalItem,
			Observacoes:              obs,
		})
	}

	// 2. Consolidação de Subtotais

	if totalCorrecao.IsNegative() && !req.AplicarDeflacao {
		totalCorrecao = decimal.Zero
	}
	subtotalCondenacao := totalPrincipalCorrigido.
		Add(totalJurosMora).
		Add(totalJurosCompensatorios).
		Add(totalMultaMoratoria).
		Add(totalCustasEDespesas).
		Sub(totalAbatimentos)
	if subtotalCondenacao.IsNegative() {
		return nil, fmt.Errorf("pagamentos superiores ao saldo não são suportados")
	}

	// 3. Encargos da Fase de Execução (Art. 523 CPC e Honorários Sucumbenciais)
	multaArt523, honorariosArt523, honorariosSucumb := CalcularEncargosExecucao(
		req.FaseExecucao,
		subtotalCondenacao,
	)

	totalMultasUnificado := totalMultaMoratoria.Add(multaArt523).Round(2)
	totalHonorariosUnificado := honorariosArt523.Add(honorariosSucumb).Round(2)
	totalJurosUnificado := totalJurosMora.Add(totalJurosCompensatorios).Round(2)

	totalGeralDevido := subtotalCondenacao.
		Add(multaArt523).
		Add(honorariosArt523).
		Add(honorariosSucumb).
		Round(2)

	// 4. Notas Metodológicas e Fundamentação
	notas := []string{
		fmt.Sprintf("Valores atualizados monetariamente até %s com base no indexador %s (%s).", dataAtualizacao.Format("02/01/2006"), indiceInfo.Nome, indiceInfo.Fonte),
	}
	if req.AplicarProRataCorrecao {
		notas = append(notas, "Correção monetária calculada pro rata die nos meses de início e término.")
	}
	if req.JurosMora.Tipo == JurosTaxaLegal || req.JurosMora.Tipo == JurosLei14905 {
		notas = append(notas, "Juros moratórios apurados em regime híbrido pela Lei Federal nº 14.905/2024 (Art. 406 do Código Civil - Taxa Legal SELIC deduzido IPCA).")
	} else if req.JurosMora.Tipo == JurosConvencionalMensal {
		modo := "simples"
		if req.JurosMora.Modo == CapitalizacaoComposta {
			modo = "compostos"
		}
		notas = append(notas, fmt.Sprintf("Juros moratórios %s fixados à razão de %s%% ao mês.", modo, req.JurosMora.PercentualMensal.String()))
	}
	if req.JurosCompensatorios.Ativo {
		notas = append(notas, fmt.Sprintf("Juros compensatórios aplicados à razão de %s%% ao mês.", req.JurosCompensatorios.TaxaMensal.StringFixed(2)))
	}
	if req.ImputarArt354CC {
		notas = append(notas, "Pagamentos parciais imputados primeiramente nos juros vencidos e o saldo no capital principal, conforme Art. 354 do Código Civil.")
	}
	if req.FaseExecucao.AplicarArt523Multa || req.FaseExecucao.AplicarArt523Honorarios {
		notas = append(notas, "Incidência dos encargos previstos no Art. 523, § 1º do Código de Processo Civil (Cumprimento de Sentença).")
	}

	return &CalculoJudicialResponse{
		OK:       true,
		Sucesso:  true,
		Mensagem: "Cálculo judicial executado com sucesso.",
		Resumo: ResumoCalculo{
			DataAtualizacao:          dataAtualizacao.Format("02/01/2006"),
			IndexadorCodigo:          req.IndexadorCodigo,
			IndexadorNome:            indiceInfo.Nome,
			TotalPrincipalOriginal:   totalPrincipalOriginal.Round(2),
			TotalPrincipalCorrigido:  totalPrincipalCorrigido.Round(2),
			TotalCorrecaoMonetaria:   totalCorrecao.Round(2),
			TotalJurosMora:           totalJurosMora.Round(2),
			TotalJuros:               totalJurosUnificado,
			TotalJurosCompensatorios: totalJurosCompensatorios.Round(2),
			TotalMultaMoratoria:      totalMultaMoratoria.Round(2),
			TotalMultas:              totalMultasUnificado,
			TotalCustasEDespesas:     totalCustasEDespesas.Round(2),
			TotalAbatimentos:         totalAbatimentos.Round(2),
			ImputadoJurosArt354:      totalImputadoJuros.Round(2),
			ImputadoPrincipalArt354:  totalImputadoPrincipal.Round(2),
			SubtotalCondenacao:       subtotalCondenacao.Round(2),
			MultaArt523CPC:           multaArt523.Round(2),
			HonorariosArt523CPC:      honorariosArt523.Round(2),
			HonorariosSucumbenciais:  honorariosSucumb.Round(2),
			TotalHonorarios:          totalHonorariosUnificado,
			TotalGeralDevido:         totalGeralDevido,
			TotalGeral:               totalGeralDevido,
		},
		ItensLiquidados:    itensLiquidados,
		Itens:              itensLiquidados,
		NotasMetodologicas: notas,
		NotasLegais:        notas,
	}, nil
}
