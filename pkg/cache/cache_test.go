package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/felipedenardo/chameleon-common/pkg/cache"
	"github.com/stretchr/testify/require"
)

type valor struct {
	Nome string `json:"nome"`
}

// Sem Redis configurado o cache precisa se comportar como se nao existisse: o
// servico sobe e responde igual, so sem economia. E o que permite ligar o cache
// em um servico por vez.
func TestResolve_SemRedisApenasConstrói(t *testing.T) {
	chamadas := 0
	got, err := cache.Resolve(context.Background(), nil,
		cache.NewKey("catalog", "servicos", "est-1"), time.Minute,
		func() (valor, error) {
			chamadas++
			return valor{Nome: "Corte"}, nil
		})

	require.NoError(t, err)
	require.Equal(t, "Corte", got.Nome)
	require.Equal(t, 1, chamadas)
}

func TestResolve_ErroDoBuildSobe(t *testing.T) {
	falha := errors.New("banco fora")
	_, err := cache.Resolve(context.Background(), nil,
		cache.NewKey("catalog", "servicos", "est-1"), time.Minute,
		func() (valor, error) { return valor{}, falha })

	require.ErrorIs(t, err, falha)
}

// A forma da chave e contrato: quem le o Redis em producao precisa conseguir
// adivinhar o que esta olhando, e a invalidacao precisa montar exatamente a
// mesma string que a leitura montou.
func TestKey_Forma(t *testing.T) {
	require.Equal(t,
		"chameleon:v1:establishment:schedule-config:est-1",
		cache.NewKey("establishment", "schedule-config", "est-1").String())

	require.Equal(t,
		"chameleon:v1:catalog:scheduling-context:est-1:g7",
		cache.NewKey("catalog", "scheduling-context", "est-1").WithGeneration(7).String())
}

// A mesma selecao pedida em ordens diferentes e a mesma pergunta. Sem
// normalizar, "corte,barba" e "barba,corte" seriam entradas distintas — o
// espaco de chaves dobraria e a taxa de acerto cairia pela metade.
func TestKey_ConjuntoIndependeDaOrdem(t *testing.T) {
	base := cache.NewKey("catalog", "scheduling-context", "est-1")
	require.Equal(t,
		base.WithSet([]string{"corte", "barba"}).String(),
		base.WithSet([]string{"barba", "corte"}).String())
}

// Chave de Redis nao e lugar para uma lista de cinquenta uuids: acima do teto
// o conjunto vira hash, mas continua estavel para a mesma entrada.
func TestKey_ConjuntoGrandeViraHash(t *testing.T) {
	base := cache.NewKey("catalog", "scheduling-context", "est-1")
	grande := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		grande = append(grande, "11111111-1111-1111-1111-11111111111"+string(rune('a'+i%26)))
	}

	primeira := base.WithSet(grande).String()
	require.Less(t, len(primeira), 120, "chave longa demais derrota o proposito")
	require.Equal(t, primeira, base.WithSet(grande).String(), "a mesma entrada precisa dar a mesma chave")
}

// WithSet e With nao podem alterar a chave de origem: uma Key montada uma vez
// e reusada para varias entidades viraria uma chave sempre crescente.
func TestKey_NaoMutaAOrigem(t *testing.T) {
	base := cache.NewKey("catalog", "servicos", "est-1")
	_ = base.With("a")
	_ = base.WithSet([]string{"b"})

	require.Equal(t, "chameleon:v1:catalog:servicos:est-1", base.String())
}
