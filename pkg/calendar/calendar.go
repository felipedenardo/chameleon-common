// Package calendar é aritmética de calendário usada igual por mais de um
// serviço. Só a conta: em que dia o ciclo de cada um cai continua decidido
// pelo serviço dono do ciclo.
package calendar

import "time"

// AddMonthsOnDay soma meses e cai no dia `day` do mês de destino — ou no
// último dia dele, quando o mês é mais curto. Diferente de time.AddDate, não
// transborda (31/01 + 1 mês é 28/02, não 03/03) e não arrasta o dia: com
// vencimento no dia 31, 28/02 + 1 mês volta a ser 31/03.
//
// A hora e o fuso de t são mantidos.
func AddMonthsOnDay(t time.Time, months, day int) time.Time {
	year, month, _ := t.Date()
	first := time.Date(year, month+time.Month(months), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	if last := first.AddDate(0, 1, -1).Day(); day > last {
		day = last
	}
	return first.AddDate(0, 0, day-1)
}
