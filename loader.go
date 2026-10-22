// Package config fornece configuração em camadas e um editor de terminal para
// aplicações Bubble Tea.
//
// As camadas são representadas por [Source] e combinadas por [Loader]. A
// prioridade decide a ordem de aplicação: valores maiores vencem. Em caso de
// empate, a camada adicionada por último vence. Mapas são mesclados
// recursivamente; listas e escalares são substituídos pela camada de maior
// prioridade.
//
// Os atalhos [Loader.Defaults], [Loader.File], [Loader.Env] e [Loader.Flags]
// usam as prioridades 0, 100, 200 e 300, respectivamente. Valores de ambiente
// são mantidos como texto na camada e convertidos apenas pelos métodos de
// leitura, como [Loader.Int] e [Loader.Duration].
//
// Arquivos inexistentes ou inválidos não viram camadas: o erro fica
// disponível em [Loader.Err]. Chaves são acessadas em notação de ponto, por
// exemplo "server.port"; uma chave ausente devolve false em [Loader.Get].
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Prioridades usadas pelos atalhos de criação de camadas.
const (
	priorityDefaults = 0
	priorityFile     = 100
	priorityEnv      = 200
	priorityFlags    = 300
)

// fileFormat identifica o formato de serialização de uma camada de arquivo.
type fileFormat int

const (
	// formatNone indica que a camada não veio de um arquivo.
	formatNone fileFormat = iota
	// formatJSON indica um arquivo JSON.
	formatJSON
	// formatYAML indica um arquivo YAML.
	formatYAML
)

// Source é uma camada de configuração, da menor para a maior prioridade.
//
// O mapa Data é copiado por [Loader.Add]; alterações posteriores no mapa do
// chamador não afetam o loader. Mapas aninhados devem usar map[string]any.
type Source struct {
	// Name identifica a camada em [Loader.Sources].
	Name string
	// Priority define a ordem de aplicação; valores maiores vencem.
	Priority int
	// Data contém os valores da camada.
	Data map[string]any

	// path e format são preenchidos apenas por [Loader.File] e usados por
	// [Editor.Save] para escolher o arquivo de destino.
	path   string
	format fileFormat
}

// Loader combina camadas de configuração.
//
// O valor devolvido por [New] é a forma recomendada de criação. Os métodos de
// construção devolvem o próprio loader para permitir encadeamento.
type Loader struct {
	sources []Source
	errs    []error
}

// New cria um loader vazio.
func New() *Loader {
	return &Loader{}
}

// Add adiciona uma camada e devolve o próprio loader.
//
// A camada é copiada no momento da adição. Camadas com a mesma prioridade são
// aplicadas na ordem em que foram adicionadas, de modo que a última vence.
func (l *Loader) Add(s Source) *Loader {
	if l == nil {
		return l
	}
	s.Data = cloneMap(s.Data)
	l.sources = append(l.sources, s)
	return l
}

// Defaults adiciona uma camada de valores padrão com prioridade 0.
func (l *Loader) Defaults(m map[string]any) *Loader {
	return l.Add(Source{Name: "defaults", Priority: priorityDefaults, Data: m})
}

// Flags adiciona uma camada de valores de linha de comando com prioridade 300.
func (l *Loader) Flags(m map[string]any) *Loader {
	return l.Add(Source{Name: "flags", Priority: priorityFlags, Data: m})
}

// File adiciona uma camada lida de um arquivo JSON ou YAML, com prioridade
// 100. A extensão define o formato: .json, .yaml ou .yml (sem diferenciar
// maiúsculas de minúsculas).
//
// Se o arquivo não existir, não puder ser lido ou tiver conteúdo inválido, a
// camada não é registrada e o erro fica disponível em [Loader.Err]. O loader
// continua utilizável; erros de chamadas anteriores permanecem acumulados.
func (l *Loader) File(path string) *Loader {
	if l == nil {
		return l
	}
	data, format, err := readConfigFile(path)
	if err != nil {
		l.errs = append(l.errs, err)
		return l
	}
	l.sources = append(l.sources, Source{
		Name:     path,
		Priority: priorityFile,
		Data:     data,
		path:     path,
		format:   format,
	})
	return l
}

// Env adiciona uma camada formada por variáveis de ambiente que começam com
// prefix+"_", com prioridade 200. O prefixo é removido, o restante é convertido
// para minúsculas e cada "_" vira um ponto: APP_DATABASE_HOST vira
// "database.host".
//
// Os valores permanecem strings na camada e são convertidos pelos métodos de
// leitura. Variáveis sem o prefixo, com sufixo vazio ou com segmentos vazios
// (por exemplo APP_A__B) são ignoradas. Um prefixo vazio considera todas as
// variáveis de ambiente.
func (l *Loader) Env(prefix string) *Loader {
	if l == nil {
		return l
	}
	data := make(map[string]any)
	entries := os.Environ()
	sort.Strings(entries)
	for _, entry := range entries {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		key, ok := envKey(prefix, name)
		if !ok {
			continue
		}
		setPath(data, key, value)
	}
	name := "env"
	if prefix != "" {
		name = "env:" + prefix
	}
	return l.Add(Source{Name: name, Priority: priorityEnv, Data: data})
}

// Err devolve os erros acumulados pelas chamadas a [Loader.File].
//
// Os erros são unidos com [errors.Join] na ordem em que ocorreram. Um loader
// sem erros devolve nil.
func (l *Loader) Err() error {
	if l == nil || len(l.errs) == 0 {
		return nil
	}
	return errors.Join(l.errs...)
}

// Get procura uma chave em notação de ponto, como "server.port", no resultado
// de [Loader.All]. O segundo valor informa se a chave existe; uma chave
// ausente, vazia ou com um segmento não mapeado devolve (nil, false).
func (l *Loader) Get(key string) (any, bool) {
	return lookup(l.All(), key)
}

// String devolve o valor textual da chave ou def se ela não existir ou não for
// uma string.
func (l *Loader) String(key, def string) string {
	v, ok := l.Get(key)
	if !ok {
		return def
	}
	s, ok := v.(string)
	if !ok {
		return def
	}
	return s
}

// Int devolve o valor inteiro da chave ou def se ela não existir ou não puder
// ser convertida. São aceitos tipos inteiros, floats integrais, json.Number e
// strings numéricas.
func (l *Loader) Int(key string, def int) int {
	v, ok := l.Get(key)
	if !ok {
		return def
	}
	n, ok := toInt(v)
	if !ok {
		return def
	}
	return n
}

// Bool devolve o valor booleano da chave ou def se ela não existir ou não
// puder ser convertida. Strings aceitam true/false, 1/0, yes/no, y/n e
// on/off, sem diferenciar maiúsculas de minúsculas.
func (l *Loader) Bool(key string, def bool) bool {
	v, ok := l.Get(key)
	if !ok {
		return def
	}
	b, ok := toBool(v)
	if !ok {
		return def
	}
	return b
}

// Duration devolve o valor de duração da chave ou def se ela não existir ou
// não puder ser convertida. Strings usam o formato de [time.ParseDuration]
// ("1s", "250ms"); valores numéricos são interpretados como nanossegundos.
func (l *Loader) Duration(key string, def time.Duration) time.Duration {
	v, ok := l.Get(key)
	if !ok {
		return def
	}
	d, ok := toDuration(v)
	if !ok {
		return def
	}
	return d
}

// All devolve a configuração mesclada.
//
// Mapas são mesclados recursivamente; listas e escalares são substituídos pela
// camada de maior prioridade. O mapa devolvido é uma cópia: alterá-lo não
// modifica o loader. A ordem de iteração de um mapa não é definida; para saída
// determinística, serialize o resultado com JSON ou YAML, que ordenam chaves.
func (l *Loader) All() map[string]any {
	out := make(map[string]any)
	for _, s := range l.orderedSources() {
		out = mergeMaps(out, s.Data)
	}
	return out
}

// Sources devolve os nomes das camadas na ordem de aplicação, da menor para a
// maior prioridade. Camadas com a mesma prioridade mantêm a ordem de inclusão.
func (l *Loader) Sources() []string {
	ordered := l.orderedSources()
	names := make([]string, 0, len(ordered))
	for _, s := range ordered {
		names = append(names, s.Name)
	}
	return names
}

// orderedSources devolve uma cópia das camadas ordenada por prioridade.
func (l *Loader) orderedSources() []Source {
	if l == nil {
		return nil
	}
	out := append([]Source(nil), l.sources...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority < out[j].Priority
	})
	return out
}

// envKey converte o nome de uma variável de ambiente em uma chave de
// configuração, respeitando o prefixo informado.
func envKey(prefix, name string) (string, bool) {
	var rest string
	switch {
	case prefix == "":
		rest = name
	case len(name) > len(prefix) &&
		strings.EqualFold(name[:len(prefix)], prefix) &&
		name[len(prefix)] == '_':
		rest = name[len(prefix)+1:]
	default:
		return "", false
	}
	if rest == "" || strings.Contains(rest, ".") {
		return "", false
	}
	parts := strings.Split(strings.ToLower(rest), "_")
	for _, part := range parts {
		if part == "" {
			return "", false
		}
	}
	return strings.Join(parts, "."), true
}

// readConfigFile lê e decodifica um arquivo de configuração.
func readConfigFile(path string) (map[string]any, fileFormat, error) {
	format, err := formatForPath(path)
	if err != nil {
		return nil, formatNone, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, formatNone, fmt.Errorf("config: arquivo %q não existe", path)
		}
		return nil, formatNone, fmt.Errorf("config: não foi possível ler %q: %w", path, err)
	}
	raw, err := decodeConfig(data, format)
	if err != nil {
		return nil, formatNone, fmt.Errorf("config: arquivo %q inválido: %w", path, err)
	}
	m, ok := raw.(map[string]any)
	if !ok || m == nil {
		return nil, formatNone, fmt.Errorf("config: arquivo %q inválido: esperado um objeto no topo", path)
	}
	return m, format, nil
}

// formatForPath identifica o formato pela extensão do arquivo.
func formatForPath(path string) (fileFormat, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return formatJSON, nil
	case ".yaml", ".yml":
		return formatYAML, nil
	default:
		return formatNone, fmt.Errorf("config: %q precisa ter extensão .json, .yaml ou .yml", path)
	}
}

// decodeConfig decodifica o conteúdo de um arquivo em um valor genérico.
func decodeConfig(data []byte, format fileFormat) (any, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("conteúdo vazio")
	}
	switch format {
	case formatJSON:
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		var raw any
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		var extra any
		if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
			if err == nil {
				return nil, errors.New("conteúdo extra após o objeto")
			}
			return nil, err
		}
		if raw == nil {
			return nil, errors.New("conteúdo vazio")
		}
		return raw, nil
	case formatYAML:
		dec := yaml.NewDecoder(bytes.NewReader(data))
		var raw any
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		var extra any
		if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
			if err == nil {
				return nil, errors.New("mais de um documento YAML")
			}
			return nil, err
		}
		if raw == nil {
			return nil, errors.New("conteúdo vazio")
		}
		return raw, nil
	default:
		return nil, errors.New("formato desconhecido")
	}
}
