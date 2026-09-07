// Package cache guarda no Redis respostas caras de montar, com chave
// versionada e invalidação explícita.
//
// # Onde o cache mora
//
// No serviço DONO do dado, não em quem o consome. O dono sabe quando o dado
// muda, então a invalidação é uma chamada local no mesmo caminho da escrita —
// sem inverter dependência entre serviços e sem janela em que a mudança já
// aconteceu e o leitor ainda não sabe.
//
// # O que isso economiza
//
// Em produção o Postgres é RDS, fora da máquina: cada consulta é uma ida à
// rede. O Redis roda na mesma instância dos serviços. Cachear uma resposta que
// custa três consultas troca três idas ao RDS por uma leitura local — o ganho
// vem daí, não de evitar o salto HTTP entre serviços, que é barato porque
// todos vivem no mesmo host.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/felipedenardo/chameleon-common/pkg/metrics"
	"github.com/redis/go-redis/v9"
)

// opTimeout é curto: o cache está no caminho da requisição, e esperar por ele
// anularia o motivo de existir. Estouro é tratado como ausência.
const opTimeout = 300 * time.Millisecond

var (
	acertos = metrics.NewCounter(
		"cache_hits_total",
		"Leituras de cache que encontraram o valor.",
		"service", "entity",
	)
	erros = metrics.NewCounter(
		"cache_misses_total",
		"Leituras de cache que não encontraram o valor ou falharam.",
		"service", "entity", "reason",
	)
)

// Store é o cache de um serviço. service rotula as métricas.
type Store struct {
	client  *redis.Client
	service string
}

func NewStore(client *redis.Client, service string) *Store {
	return &Store{client: client, service: service}
}

// Enabled diz se há Redis configurado. Store nulo é um cache que não guarda
// nada — quem chama não precisa de condicional, e o serviço sobe sem Redis.
func (s *Store) Enabled() bool {
	return s != nil && s.client != nil
}

// Resolve devolve o valor guardado ou executa build e guarda o resultado.
//
// Falha do Redis NUNCA vira erro para quem chamou: cache indisponível deve
// custar desempenho, não disponibilidade — o serviço volta a fazer o que fazia
// antes de existir cache. O que se perde é registrado em cache_misses_total,
// com o motivo, para que "o Redis está fora" não passe por "os dados mudam
// muito" na hora de ler o painel.
//
// Erro de build sobe e NÃO é guardado: cachear falha transitória a
// transformaria em falha determinística pela duração do TTL.
func Resolve[T any](ctx context.Context, s *Store, key Key, ttl time.Duration, build func() (T, error)) (T, error) {
	if !s.Enabled() {
		return build()
	}

	entidade := key.entity
	chave := key.String()

	lerCtx, cancelarLeitura := context.WithTimeout(ctx, opTimeout)
	bruto, err := s.client.Get(lerCtx, chave).Bytes()
	cancelarLeitura()

	switch {
	case err == nil:
		var guardado T
		if jsonErr := json.Unmarshal(bruto, &guardado); jsonErr == nil {
			acertos.WithLabelValues(s.service, entidade).Inc()
			return guardado, nil
		}
		// Valor ilegível é tratado como ausência: quem gravou usava outro
		// formato, e a versão de schema na chave existe justamente para isso
		// não acontecer. Se acontecer, remonta em vez de derrubar a resposta.
		erros.WithLabelValues(s.service, entidade, "corrompido").Inc()
	case errors.Is(err, redis.Nil):
		erros.WithLabelValues(s.service, entidade, "ausente").Inc()
	default:
		erros.WithLabelValues(s.service, entidade, "indisponivel").Inc()
	}

	valor, err := build()
	if err != nil {
		return valor, err
	}

	if bytes, marshalErr := json.Marshal(valor); marshalErr == nil {
		gravarCtx, cancelarGravacao := context.WithTimeout(ctx, opTimeout)
		_ = s.client.Set(gravarCtx, chave, bytes, ttl).Err()
		cancelarGravacao()
	}

	return valor, nil
}

// Forget apaga chaves. Chamado no caminho da ESCRITA do dono do dado.
//
// O erro é devolvido para quem chama decidir. Falhar em invalidar não corrompe
// nada, mas deixa o dado velho no ar até o TTL — quem acabou de salvar é a
// primeira pessoa a reparar, então engolir isso em silêncio seria pior.
func (s *Store) Forget(ctx context.Context, keys ...Key) error {
	if !s.Enabled() || len(keys) == 0 {
		return nil
	}

	nomes := make([]string, 0, len(keys))
	for _, key := range keys {
		nomes = append(nomes, key.String())
	}

	opCtx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return s.client.Del(opCtx, nomes...).Err()
}

// Generation devolve o número de geração atual de um escopo, usado por chaves
// que não dão para apagar uma a uma.
//
// Custa uma leitura a mais por consulta ao cache: primeiro a geração, depois o
// dado. Localmente são décimos de milissegundo, contra os round-trips ao RDS
// que a consulta substitui.
//
// Ausência devolve 0 e não é erro: escopo que nunca foi invalidado está na
// geração zero. Redis fora também devolve 0 — todo mundo lendo a mesma geração
// errada ainda é consistente entre si, e o TTL continua limitando o estrago.
//
// # Dependência invisível: a chave de geração não pode ser despejada
//
// Se ela sumir, a leitura volta a zero e passa a montar chaves da primeira
// geração — que podem existir ainda, guardando dado anterior a todas as
// invalidações. O estrago é limitado pelo TTL dos dados (degrada para
// invalidação só por tempo, que é a linha de base sem geração nenhuma), mas
// não é o comportamento desejado.
//
// Hoje isso não acontece porque o Redis roda com `maxmemory 0` e
// `maxmemory-policy noeviction`: nada é despejado por pressão de memória, e o
// AOF sobrevive a restart. Ligar um limite de memória com política `allkeys-*`
// torna esta chave candidata a despejo como qualquer outra, e a invalidação
// passa a falhar em silêncio.
func (s *Store) Generation(ctx context.Context, scope Key) int64 {
	if !s.Enabled() {
		return 0
	}

	opCtx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()

	valor, err := s.client.Get(opCtx, scope.String()).Int64()
	if err != nil {
		return 0
	}
	return valor
}

// NextGeneration incrementa a geração de um escopo, tornando inalcançável toda
// chave montada com a anterior.
//
// É a invalidação de espaço de chaves combinatório: uma operação, em vez de
// varrer o Redis atrás de tudo que casa com um padrão. As chaves órfãs somem
// sozinhas pelo TTL.
func (s *Store) NextGeneration(ctx context.Context, scope Key) error {
	if !s.Enabled() {
		return nil
	}

	opCtx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	return s.client.Incr(opCtx, scope.String()).Err()
}
