package persistence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/felipedenardo/chameleon-common/pkg/persistence"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A implementação real (gormUnitOfWork) abre uma transação de verdade via
// *gorm.DB.Transaction — não dá pra exercitar o caminho de sucesso/rollback
// sem Postgres real (mesma exceção documentada em go-testing.md pra qualquer
// código que abre *gorm.DB direto). O que é testável aqui, com mock puro, é
// o Noop (usado pelos testes de domain/application) e o fallback do DB().

func TestNoopUnitOfWork_ChamaFnComOMesmoContexto(t *testing.T) {
	uow := persistence.NewNoopUnitOfWork()
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "valor")

	var gotCtx context.Context
	err := uow.Execute(ctx, func(fnCtx context.Context) error {
		gotCtx = fnCtx
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, "valor", gotCtx.Value(ctxKey{}))
}

func TestNoopUnitOfWork_PropagaErroDeFn(t *testing.T) {
	uow := persistence.NewNoopUnitOfWork()
	fnErr := errors.New("segundo passo falhou")

	err := uow.Execute(context.Background(), func(context.Context) error {
		return fnErr
	})

	require.ErrorIs(t, err, fnErr)
}

func TestDB_SemTransacaoEmAndamentoDevolveOFallback(t *testing.T) {
	fallback := &gorm.DB{}

	got := persistence.DB(context.Background(), fallback)

	require.Same(t, fallback, got)
}
