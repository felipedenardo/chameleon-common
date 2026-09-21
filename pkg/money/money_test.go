package money_test

import (
	"testing"

	"github.com/felipedenardo/chameleon-common/pkg/money"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func dec(v string) decimal.Decimal { return decimal.RequireFromString(v) }

func TestRound(t *testing.T) {
	for _, tc := range []struct{ entrada, esperado string }{
		{"4.9995", "5"},
		{"28.3305", "28.33"},
		{"0.085", "0.09"},
		{"2.344", "2.34"},
		{"2.345", "2.35"},
		{"10", "10"},
	} {
		t.Run(tc.entrada, func(t *testing.T) {
			require.True(t, money.Round(dec(tc.entrada)).Equal(dec(tc.esperado)),
				"esperava %s, veio %s", tc.esperado, money.Round(dec(tc.entrada)))
		})
	}
}

func TestApplyPercentDiscount(t *testing.T) {
	for _, tc := range []struct{ nome, lista, percent, esperado string }{
		{"quinze por cento redondo", "40", "15", "34"},
		{"quinze por cento com centavo", "33.33", "15", "28.33"},
		{"cem por cento zera", "40", "100", "0"},
		{"zero por cento nao muda", "40", "0", "40"},
		{"acima de cem nao devolve dinheiro", "40", "150", "0"},
		{"lista negativa nao vira credito", "-10", "15", "0"},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			got := money.ApplyPercentDiscount(dec(tc.lista), dec(tc.percent))
			require.True(t, got.Equal(dec(tc.esperado)), "esperava %s, veio %s", tc.esperado, got)
		})
	}
}

// A ordem do arredondamento muda o resultado, e a regra é arredondar o PREÇO.
// Este é o caso que separa as duas: arredondando o desconto daria 0,08.
func TestApplyPercentDiscount_ArredondaOPrecoNaoODesconto(t *testing.T) {
	efetivo := money.ApplyPercentDiscount(dec("0.10"), dec("15"))

	require.True(t, efetivo.Equal(dec("0.09")),
		"arredondar o desconto daria 0.08; a regra do ecossistema arredonda o preco")
}

func TestApplyAmountDiscount(t *testing.T) {
	for _, tc := range []struct{ nome, lista, amount, esperado string }{
		{"abatimento comum", "40", "6", "34"},
		{"abatimento maior que o preco tem piso em zero", "5", "10", "0"},
		{"abatimento igual ao preco zera", "40", "40", "0"},
		{"abatimento zero nao muda", "40", "0", "40"},
		{"centavos", "33.33", "0.335", "33"},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			got := money.ApplyAmountDiscount(dec(tc.lista), dec(tc.amount))
			require.True(t, got.Equal(dec(tc.esperado)), "esperava %s, veio %s", tc.esperado, got)
		})
	}
}

// O abatimento nunca é guardado: sai de (tabela - efetivo). Assim os dois
// números nunca se contradizem, qualquer que seja o desconto.
func TestAbatimentoEDerivado(t *testing.T) {
	lista := dec("33.33")
	efetivo := money.ApplyPercentDiscount(lista, dec("15"))

	require.True(t, lista.Sub(efetivo).Equal(dec("5")),
		"33.33 - 28.33 = 5.00, exato por construcao")
}

func TestApplyDiscount(t *testing.T) {
	for _, tc := range []struct {
		nome, lista, kind, valor, esperado string
		conhecida                          bool
	}{
		{nome: "percentual despacha para o calculo percentual", lista: "33.33", kind: money.DiscountPercent, valor: "15", esperado: "28.33", conhecida: true},
		{nome: "quantia despacha para o calculo em dinheiro", lista: "40", kind: money.DiscountAmount, valor: "6", esperado: "34", conhecida: true},
		{nome: "regra desconhecida cobra o preco cheio", lista: "40", kind: "percentage", valor: "15", esperado: "40", conhecida: false},
		{nome: "regra vazia cobra o preco cheio", lista: "40", kind: "", valor: "15", esperado: "40", conhecida: false},
	} {
		t.Run(tc.nome, func(t *testing.T) {
			got, conhecida := money.ApplyDiscount(dec(tc.lista), tc.kind, dec(tc.valor))
			require.Equal(t, tc.conhecida, conhecida, "segundo retorno diz se a regra era conhecida")
			require.True(t, got.Equal(dec(tc.esperado)), "esperava %s, veio %s", tc.esperado, got)
		})
	}
}

// O despacho não pode divergir das duas funções que ele chama: é justamente a
// divergência entre cópias do mesmo cálculo que o pacote existe para evitar.
func TestApplyDiscountNaoDivergeDoCalculoDireto(t *testing.T) {
	for _, lista := range []string{"0.10", "33.33", "40", "199.99"} {
		for _, valor := range []string{"0", "15", "100"} {
			porcento, _ := money.ApplyDiscount(dec(lista), money.DiscountPercent, dec(valor))
			require.True(t, porcento.Equal(money.ApplyPercentDiscount(dec(lista), dec(valor))),
				"percentual de %s sobre %s divergiu", valor, lista)

			quantia, _ := money.ApplyDiscount(dec(lista), money.DiscountAmount, dec(valor))
			require.True(t, quantia.Equal(money.ApplyAmountDiscount(dec(lista), dec(valor))),
				"quantia de %s sobre %s divergiu", valor, lista)
		}
	}
}
