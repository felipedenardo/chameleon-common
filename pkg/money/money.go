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

// Vocabulário das regras de desconto da plataforma. É a string que o plano do
// clube publica, que trafega entre os serviços e que fica gravada na venda.
//
// Mora aqui junto do cálculo porque quatro serviços precisavam da mesma
// palavra: a mesma string escrita em quatro arquivos diverge no dia em que
// alguém digita "percentage" num deles, e o desconto simplesmente deixa de
// aplicar sem ninguém errar um teste.
const (
	DiscountPercent = "percent"
	DiscountAmount  = "amount"
)

// ApplyDiscount devolve o preço efetivo depois da regra pedida, e se a regra
// era conhecida.
//
// Existe porque o despacho entre percentual e quantia estava escrito em três
// lugares (dois no agendamento, um na venda), e cada cópia é uma chance de o
// centavo divergir entre a tela que mostra e o caixa que cobra.
//
// Regra desconhecida devolve o próprio preço de tabela, e não zero: assim, quem
// ignorar o segundo retorno cobra o preço cheio em vez de dar o item de graça.
// O `false` está aí para quem precisa distinguir "o plano não descontou" de
// "o plano mandou uma regra que este código não entende" -- o segundo é dado
// corrompido, e cabe a quem chama recusar ou seguir.
func ApplyDiscount(list decimal.Decimal, kind string, value decimal.Decimal) (decimal.Decimal, bool) {
	switch kind {
	case DiscountPercent:
		return ApplyPercentDiscount(list, value), true
	case DiscountAmount:
		return ApplyAmountDiscount(list, value), true
	default:
		return list, false
	}
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
