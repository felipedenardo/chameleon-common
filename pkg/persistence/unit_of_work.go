// Package persistence dá ao domain/application uma forma de rodar operações
// multi-passo (2+ writes em entidades diferentes) numa única transação SEM
// depender de *gorm.DB diretamente. O service nunca abre transação nem chama
// GORM: ele chama UnitOfWork.Execute, que abre a transação e injeta a conexão
// no context; os repositórios pegam essa conexão via DB(ctx, fallback) — se
// não houver transação em andamento, usam a conexão base normalmente.
package persistence

import (
	"context"

	"gorm.io/gorm"
)

// Mock exportado (não fica em internal/) porque UnitOfWork é consumido pelos
// outros repos do ecossistema — cada service não precisa gerar sua própria
// cópia, importa direto de chameleon-common/pkg/persistence/mocks.
//
//go:generate mockgen -source=$GOFILE -destination=mocks/unit_of_work_mock.go -package=mocks -mock_names UnitOfWork=MockUnitOfWork
type UnitOfWork interface {
	// Execute roda fn dentro de uma transação: erro (ou panic) de fn faz
	// rollback, sucesso faz commit. fn recebe o ctx já carregando a conexão
	// transacional — repositórios chamados com esse ctx participam da mesma
	// transação automaticamente, sem receber *gorm.DB como parâmetro.
	Execute(ctx context.Context, fn func(ctx context.Context) error) error
}

type gormUnitOfWork struct {
	db *gorm.DB
}

// NewUnitOfWork é a implementação real, usada em produção via DI.
func NewUnitOfWork(db *gorm.DB) UnitOfWork {
	return &gormUnitOfWork{db: db}
}

func (u *gormUnitOfWork) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTx(ctx, tx))
	})
}

// noopUnitOfWork roda fn direto, sem transação nem banco algum — pra teste
// unitário de domain/application, onde os repositórios já são mockados e a
// orquestração (ordem das chamadas, propagação de erro do segundo passo) é o
// que se quer exercitar, sem precisar de Postgres real.
type noopUnitOfWork struct{}

// NewNoopUnitOfWork é o padrão em testes: chama fn com o ctx original, sem
// abrir transação. Repositórios mockados não leem conexão nenhuma do ctx, só
// os reais (via DB) fariam isso — então é seguro pra qualquer teste que mocka
// IRepository/IService.
func NewNoopUnitOfWork() UnitOfWork {
	return &noopUnitOfWork{}
}

func (noopUnitOfWork) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type txKey struct{}

func withTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// DB devolve a conexão transacional do ctx (se Execute estiver em andamento)
// ou fallback caso contrário. Todo método de repositório usa isso no lugar de
// referenciar o campo db diretamente — é isso que torna Create/Update/Delete
// tx-aware sem precisar de uma variante *Tx(tx *gorm.DB, ...) na interface.
func DB(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return fallback
}
