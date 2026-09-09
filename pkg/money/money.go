// Package money centraliza a aritmética de dinheiro do ecossistema.
//
// Existe por uma razão só: o resultado de um desconto precisa ser idêntico em
// todo lugar que o calcule. Duas implementações do mesmo desconto divergem em
// um centavo mais cedo ou mais tarde, e aí o valor exibido antes de confirmar
// não bate com o valor cobrado.
//
// Mora na biblioteca compartilhada de propósito: no dia em que um segundo
// serviço precisar aplicar desconto, ele importa daqui em vez de reimplementar
// — e é a reimplementação que faz o centavo divergir.
package money

import "github.com/shopspring/decimal"

// Scale é a precisão de dinheiro: centavos.
const Scale = 2

var cem = decimal.NewFromInt(100)

// Round arredonda para centavos, meio para cima.
//
// Meio para cima, e não truncando: truncar sempre para baixo é viés numa
// direção só, e em volume o estabelecimento perde um valor previsível. Meio
// para cima é simétrico e é o que sistema fiscal faz, o que evita divergência
// se nota fiscal entrar na história.
func Round(value decimal.Decimal) decimal.Decimal {
	return value.Round(Scale)
}

// ApplyPercentDiscount devolve o preço efetivo depois de um desconto
// percentual.
//
// Arredonda o PREÇO, nunca o desconto. As duas ordens divergem: 15% de R$0,10
// dá R$0,09 arredondando o preço e R$0,08 arredondando o desconto. O preço é
// o número que o cliente paga e o que fica gravado, então é ele que recebe o
// arredondamento; o abatimento é sempre derivado de (tabela - efetivo).
func ApplyPercentDiscount(list, percent decimal.Decimal) decimal.Decimal {
	if list.IsNegative() {
		return decimal.Zero
	}
	restante := cem.Sub(percent)
	if restante.IsNegative() {
		// Desconto acima de cem por cento devolveria dinheiro ao cliente.
		return decimal.Zero
	}
	return floorAtZero(Round(list.Mul(restante).Div(cem)))
}

// ApplyAmountDiscount devolve o preço efetivo depois de um desconto em
// dinheiro.
//
// Piso em zero: R$10 de abatimento num produto de R$5 resulta em R$0, nunca
// num valor negativo que o estabelecimento pagaria para vender.
func ApplyAmountDiscount(list, amount decimal.Decimal) decimal.Decimal {
	return floorAtZero(Round(list.Sub(amount)))
}

func floorAtZero(value decimal.Decimal) decimal.Decimal {
	if value.IsNegative() {
		return decimal.Zero
	}
	return value
}
