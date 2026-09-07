package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// prefix e schemaVersion abrem toda chave do cache.
//
// O prefixo separa este cache de qualquer outra coisa no mesmo Redis (a lista
// de revogação do auth mora lá também). A versão de schema é o que permite
// mudar o FORMATO de um valor guardado sem envenenar quem já está no ar:
// subindo a versão, as chaves antigas ficam inalcançáveis e morrem pelo TTL,
// em vez de serem lidas como um tipo que não são mais.
const (
	prefix        = "chameleon"
	schemaVersion = "v1"
)

// Key é uma chave de cache montada por partes, em vez de string solta.
//
// Chave montada com fmt.Sprintf espalhado pelo código diverge — um esquece o
// prefixo, outro troca a ordem, e o resultado é cache que nunca acerta e
// invalidação que não invalida. Aqui a forma é uma só:
//
//	chameleon:v1:<domínio>:<entidade>[:<parte>...]
type Key struct {
	domain string
	entity string
	parts  []string
}

// NewKey abre uma chave. domain é o serviço dono do dado (establishment,
// catalog); entity é o que está guardado (schedule-config, scheduling-context).
func NewKey(domain, entity string, parts ...string) Key {
	return Key{domain: domain, entity: entity, parts: parts}
}

// With acrescenta partes à chave, devolvendo uma nova — a original não muda.
func (k Key) With(parts ...string) Key {
	combinado := make([]string, 0, len(k.parts)+len(parts))
	combinado = append(combinado, k.parts...)
	combinado = append(combinado, parts...)
	return Key{domain: k.domain, entity: k.entity, parts: combinado}
}

// WithGeneration acrescenta o número de geração do dono do dado.
//
// É o que torna a invalidação viável quando o espaço de chaves é combinatório
// (uma chave por combinação de serviços escolhidos, por exemplo): apagar por
// padrão exigiria varrer o Redis, e incrementar a geração torna TODAS as
// chaves anteriores inalcançáveis de uma vez. As órfãs somem pelo TTL.
func (k Key) WithGeneration(generation int64) Key {
	return k.With("g" + strconv.FormatInt(generation, 10))
}

// WithSet acrescenta um conjunto de valores como UMA parte, independente da
// ordem em que vieram.
//
// Ordena antes de juntar porque a mesma seleção pedida em ordens diferentes é
// a mesma pergunta — sem isso "corte,barba" e "barba,corte" seriam entradas
// separadas, dobrando o espaço de chaves e cortando a taxa de acerto pela
// metade. Conjunto grande vira hash: chave de Redis não é lugar para uma lista
// de cinquenta uuids.
func (k Key) WithSet(values []string) Key {
	ordenados := make([]string, len(values))
	copy(ordenados, values)
	sort.Strings(ordenados)
	junto := strings.Join(ordenados, ",")

	if len(junto) <= 120 {
		return k.With(junto)
	}
	soma := sha256.Sum256([]byte(junto))
	return k.With("h" + hex.EncodeToString(soma[:12]))
}

// String devolve a chave como o Redis a vê.
func (k Key) String() string {
	partes := make([]string, 0, len(k.parts)+4)
	partes = append(partes, prefix, schemaVersion, k.domain, k.entity)
	partes = append(partes, k.parts...)
	return strings.Join(partes, ":")
}
