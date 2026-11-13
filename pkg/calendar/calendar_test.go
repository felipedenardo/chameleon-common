package calendar_test

import (
	"testing"
	"time"

	"github.com/felipedenardo/chameleon-common/pkg/calendar"
	"github.com/stretchr/testify/require"
)

// time.AddDate transborda (31/01 + 1 mês = 03/03); ciclo de cobrança não pode
// pular o mês inteiro de fevereiro, nem arrastar o dia de vencimento.
func TestAddMonthsOnDay(t *testing.T) {
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 9, 30, 0, 0, time.UTC) }
	for _, tc := range []struct {
		de       time.Time
		meses    int
		dia      int
		esperado time.Time
	}{
		{d(2027, 1, 31), 1, 31, d(2027, 2, 28)},
		{d(2028, 1, 31), 1, 31, d(2028, 2, 29)},
		{d(2027, 2, 28), 1, 31, d(2027, 3, 31)},
		{d(2026, 11, 30), 3, 31, d(2027, 2, 28)},
		{d(2026, 8, 31), 6, 31, d(2027, 2, 28)},
		{d(2026, 10, 3), 12, 3, d(2027, 10, 3)},
		{d(2026, 3, 15), 1, 15, d(2026, 4, 15)},
		{d(2026, 12, 15), 1, 15, d(2027, 1, 15)},
		{d(2026, 12, 31), 2, 31, d(2027, 2, 28)},
	} {
		require.Equal(t, tc.esperado, calendar.AddMonthsOnDay(tc.de, tc.meses, tc.dia),
			"%s + %d mes(es), dia %d", tc.de, tc.meses, tc.dia)
	}
}

// O fuso de quem chama fica: o ciclo do clube e o da plataforma contam no
// calendário de São Paulo.
func TestAddMonthsOnDay_MantemFusoEHora(t *testing.T) {
	sp, err := time.LoadLocation("America/Sao_Paulo")
	require.NoError(t, err)
	de := time.Date(2027, 1, 31, 0, 0, 0, 0, sp)

	got := calendar.AddMonthsOnDay(de, 1, 31)

	require.Equal(t, sp, got.Location())
	require.Equal(t, time.Date(2027, 2, 28, 0, 0, 0, 0, sp), got)
}
