package calculator

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

var (
	ErrIndiceNaoEncontrado = errors.New("indice economico nao encontrado")
	ErrDataForaDaSerie     = errors.New("data solicitada esta fora do periodo coberto pela serie historica")
)

// RepositorioIndices gerencia as séries históricas de índices econômicos em memória com thread-safety
type RepositorioIndices struct {
	mu      sync.RWMutex
	indices map[string]IndiceInfo
	// series: map[codigo_indice]map[ano_mes_string]PontoSerie, ex: "2024-01"
	series map[string]map[string]PontoSerie
}

func NovoRepositorioIndices() *RepositorioIndices {
	repo := &RepositorioIndices{
		indices: make(map[string]IndiceInfo),
		series:  make(map[string]map[string]PontoSerie),
	}
	repo.RegistrarIndice(IndiceInfo{Codigo: "SEM_CORRECAO", Nome: "Sem correção monetária", Fonte: "Configuração explícita", Periodicidade: "NENHUMA"})
	return repo
}

// RegistrarIndice cadastra os metadados de um novo índice
func (r *RepositorioIndices) RegistrarIndice(info IndiceInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.indices[info.Codigo] = info
	if _, ok := r.series[info.Codigo]; !ok {
		r.series[info.Codigo] = make(map[string]PontoSerie)
	}
}

// AdicionarPonto insere ou atualiza um ponto na série histórica
func (r *RepositorioIndices) AdicionarPonto(codigoIndice string, dataRef time.Time, valorPct, numeroIndice decimal.Decimal) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.series[codigoIndice]; !ok {
		r.series[codigoIndice] = make(map[string]PontoSerie)
	}

	chaveAnoMes := dataRef.Format("2006-01")
	fator := decimal.NewFromInt(1).Add(valorPct.Div(decimal.NewFromInt(100)))

	ponto := PontoSerie{
		DataReferencia:      dataRef,
		DataReferenciaTexto: chaveAnoMes,
		ValorPercentual:     valorPct,
		FatorMultiplicador:  fator,
		NumeroIndice:        numeroIndice,
	}

	r.series[codigoIndice][chaveAnoMes] = ponto
}

// ObterIndice retorna as informações cadastrais do índice
func (r *RepositorioIndices) ObterIndice(codigo string) (IndiceInfo, error) {
	codigo = NormalizarCodigoIndice(codigo)
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.indices[codigo]
	if !ok {
		return IndiceInfo{}, fmt.Errorf("%w: %s", ErrIndiceNaoEncontrado, codigo)
	}
	return info, nil
}

// ListarIndices retorna a lista ordenada de todos os índices disponíveis
func (r *RepositorioIndices) ListarIndices() []IndiceInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	lista := make([]IndiceInfo, 0, len(r.indices))
	for _, info := range r.indices {
		lista = append(lista, info)
	}
	sort.Slice(lista, func(i, j int) bool {
		return lista[i].Codigo < lista[j].Codigo
	})
	return lista
}

// ObterPonto busca o registro específico de um mês
func (r *RepositorioIndices) ObterPonto(codigo string, data time.Time) (PontoSerie, error) {
	codigo = NormalizarCodigoIndice(codigo)
	r.mu.RLock()
	defer r.mu.RUnlock()

	serie, ok := r.series[codigo]
	if !ok {
		return PontoSerie{}, fmt.Errorf("%w: %s", ErrIndiceNaoEncontrado, codigo)
	}

	chaveAnoMes := data.Format("2006-01")
	ponto, ok := serie[chaveAnoMes]
	if !ok {
		return PontoSerie{}, fmt.Errorf("%w: %s para o mes %s", ErrDataForaDaSerie, codigo, chaveAnoMes)
	}

	return ponto, nil
}

// ObterFatorAcumulado calcula o fator de correção entre duas datas
func (r *RepositorioIndices) ObterFatorAcumulado(codigo string, dataInicio, dataFim time.Time, permitirDeflacao bool) (decimal.Decimal, error) {
	return r.ObterFatorAcumuladoComOpcoes(codigo, dataInicio, dataFim, permitirDeflacao, false)
}

// ObterFatorAcumuladoComOpcoes calcula o fator de correção entre duas datas com suporte a pro rata die opcional
func (r *RepositorioIndices) ObterFatorAcumuladoComOpcoes(codigo string, dataInicio, dataFim time.Time, permitirDeflacao, aplicarProRata bool) (decimal.Decimal, error) {
	codigo = NormalizarCodigoIndice(codigo)
	if dataInicio.After(dataFim) {
		return decimal.Zero, fmt.Errorf("data inicial posterior à final")
	}
	if codigo == "SEM_CORRECAO" {
		return decimal.NewFromInt(1), nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	serie, ok := r.series[codigo]
	if !ok {
		return decimal.NewFromInt(1), fmt.Errorf("%w: %s", ErrIndiceNaoEncontrado, codigo)
	}

	if !dataInicio.Before(dataFim) {
		return decimal.NewFromInt(1), nil
	}

	pontoInicio, okInicio := serie[dataInicio.Format("2006-01")]
	pontoFim, okFim := serie[dataFim.Format("2006-01")]

	if !okInicio || !okFim {
		return decimal.NewFromInt(1), fmt.Errorf("%w: periodo %s ate %s para indice %s",
			ErrDataForaDaSerie, dataInicio.Format("2006-01"), dataFim.Format("2006-01"), codigo)
	}

	// 1. Caso com deflação permitida, sem pro-rata fracionado e número-índice presente: quociente direto
	if permitirDeflacao && !aplicarProRata && !pontoInicio.NumeroIndice.IsZero() && !pontoFim.NumeroIndice.IsZero() {
		return pontoFim.NumeroIndice.DivRound(pontoInicio.NumeroIndice, 10), nil
	}

	// 2. Caso sem deflação, sem número-índice ou com pro rata die na correção monetária
	fatorTotal := decimal.NewFromInt(1)
	cento := decimal.NewFromInt(100)
	trinta := decimal.NewFromInt(30)

	// Se aplicarProRata estiver ativo, aplica fração proporcional de dias no mês inicial
	if aplicarProRata {
		diasMesInicial := 30 - min(dataInicio.Day(), 30)
		if dataInicio.Year() == dataFim.Year() && dataInicio.Month() == dataFim.Month() {
			diasMesInicial = min(dataFim.Day(), 30) - min(dataInicio.Day(), 30)
		}
		if diasMesInicial < 0 {
			diasMesInicial = 0
		}
		varInicial := pontoInicio.ValorPercentual
		if !permitirDeflacao && varInicial.IsNegative() {
			varInicial = decimal.Zero
		}
		proporcaoIni := decimal.NewFromInt(int64(diasMesInicial)).Div(trinta)
		fatorIni := decimal.NewFromInt(1).Add(varInicial.Mul(proporcaoIni).Div(cento))
		fatorTotal = fatorTotal.Mul(fatorIni)
	}

	cursor := time.Date(dataInicio.Year(), dataInicio.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	fimMes := time.Date(dataFim.Year(), dataFim.Month(), 1, 0, 0, 0, 0, time.UTC)

	for !cursor.After(fimMes) {
		chave := cursor.Format("2006-01")
		ponto, ok := serie[chave]
		if !ok {
			return decimal.Zero, fmt.Errorf("%w: %s em %s", ErrDataForaDaSerie, codigo, chave)
		}
		if ok {
			variacao := ponto.ValorPercentual
			if !permitirDeflacao && variacao.IsNegative() {
				variacao = decimal.Zero
			}

			// Se for o último mês e aplicarProRata estiver ativo, proporcionaliza pelos dias corridos do mês final
			if aplicarProRata && cursor.Equal(fimMes) {
				diasFim := dataFim.Day()
				if diasFim > 30 {
					diasFim = 30
				}
				proporcaoFim := decimal.NewFromInt(int64(diasFim)).Div(trinta)
				variacao = variacao.Mul(proporcaoFim)
			}

			fatorMes := decimal.NewFromInt(1).Add(variacao.Div(cento))
			fatorTotal = fatorTotal.Mul(fatorMes)
		}
		cursor = cursor.AddDate(0, 1, 0)
	}

	return fatorTotal.Round(10), nil
}

// ObterSerieCompleta retorna todos os pontos ordenados cronologicamente para exibição
func (r *RepositorioIndices) ObterSerieCompleta(codigo string) ([]PontoSerie, error) {
	codigo = NormalizarCodigoIndice(codigo)
	r.mu.RLock()
	defer r.mu.RUnlock()

	serie, ok := r.series[codigo]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrIndiceNaoEncontrado, codigo)
	}

	lista := make([]PontoSerie, 0, len(serie))
	for _, p := range serie {
		lista = append(lista, p)
	}
	sort.Slice(lista, func(i, j int) bool {
		return lista[i].DataReferenciaTexto < lista[j].DataReferenciaTexto
	})
	return lista, nil
}
