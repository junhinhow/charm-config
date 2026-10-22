package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestDeepMergeTresCamadas cobre merge recursivo, substituição de lista e de
// escalar entre três camadas.
func TestDeepMergeTresCamadas(t *testing.T) {
	defaults := map[string]any{
		"server": map[string]any{
			"host": "localhost",
			"port": 8080,
			"tls":  false,
			"tags": []any{"default", "antigo"},
		},
		"log": map[string]any{"level": "info", "format": "text"},
	}
	loader := New().
		Defaults(defaults).
		Add(Source{
			Name:     "arquivo",
			Priority: priorityFile,
			Data: map[string]any{
				"server": map[string]any{"host": "db.interno", "tags": []any{"arquivo"}},
				"log":    map[string]any{"level": "debug"},
			},
		}).
		Flags(map[string]any{
			"server": map[string]any{"port": 9090},
		})

	all := loader.All()
	server, ok := all["server"].(map[string]any)
	if !ok {
		t.Fatalf("server deveria ser map[string]any, obteve %T", all["server"])
	}
	if got := server["host"]; got != "db.interno" {
		t.Errorf("host = %v, esperado db.interno", got)
	}
	if got := server["port"]; got != 9090 {
		t.Errorf("port = %v, esperado 9090", got)
	}
	if got := server["tls"]; got != false {
		t.Errorf("tls = %v, esperado false", got)
	}
	tags, ok := server["tags"].([]any)
	if !ok {
		t.Fatalf("tags deveria ser []any, obteve %T", server["tags"])
	}
	if !reflect.DeepEqual(tags, []any{"arquivo"}) {
		t.Errorf("tags = %#v, esperado somente a lista da camada mais alta", tags)
	}
	logMap, ok := all["log"].(map[string]any)
	if !ok {
		t.Fatalf("log deveria ser map[string]any, obteve %T", all["log"])
	}
	if logMap["level"] != "debug" || logMap["format"] != "text" {
		t.Errorf("log = %#v, esperado level=debug e format=text", logMap)
	}

	// A cópia devolvida não pode compartilhar estado com o loader.
	server["port"] = 1
	if got := loader.Int("server.port", 0); got != 9090 {
		t.Errorf("All devolveu mapa compartilhado: port = %d", got)
	}
	// A camada original também foi copiada em Add.
	defaults["server"].(map[string]any)["host"] = "alterado"
	if got := loader.String("server.host", ""); got != "db.interno" {
		t.Errorf("Add não copiou a camada: host = %q", got)
	}

	// Slices tipados também são copiados.
	ints := []int{1, 2}
	typed := New().Defaults(map[string]any{"ints": ints})
	ints[0] = 9
	if got := typed.All()["ints"].([]int)[0]; got != 1 {
		t.Errorf("slice tipado compartilhado: %d", got)
	}

	// Mapas com tipo concreto também participam do merge profundo.
	typedMaps := New().
		Defaults(map[string]any{"server": map[string]string{"host": "a", "port": "1"}}).
		Flags(map[string]any{"server": map[string]any{"port": "2"}})
	if got := typedMaps.String("server.host", ""); got != "a" {
		t.Errorf("host de mapa tipado = %q, esperado a", got)
	}
	if got := typedMaps.String("server.port", ""); got != "2" {
		t.Errorf("port de mapa tipado = %q, esperado 2", got)
	}

	// Slices de mapas tipados não devem causar panic na cópia.
	typedList := New().Defaults(map[string]any{
		"items": []map[string]string{{"a": "b"}},
	})
	items, ok := typedList.All()["items"].([]map[string]string)
	if !ok || len(items) != 1 || items[0]["a"] != "b" {
		t.Errorf("lista de mapas tipados = %#v", typedList.All()["items"])
	}
}

// TestPrioridadeEmpate cobre a ordem estável em caso de prioridade igual.
func TestPrioridadeEmpate(t *testing.T) {
	loader := New().
		Add(Source{Name: "a", Priority: 100, Data: map[string]any{"valor": "a"}}).
		Add(Source{Name: "b", Priority: 100, Data: map[string]any{"valor": "b"}}).
		Add(Source{Name: "baixa", Priority: 50, Data: map[string]any{"outro": true}})

	if got := loader.String("valor", ""); got != "b" {
		t.Errorf("valor = %q, esperado b", got)
	}
	want := []string{"baixa", "a", "b"}
	if got := loader.Sources(); !reflect.DeepEqual(got, want) {
		t.Errorf("Sources = %#v, esperado %#v", got, want)
	}
}

// TestPrioridadeDosAtalhos cobre a ordem defaults < arquivo < env < flags.
func TestPrioridadeDosAtalhos(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_SERVER_PORT", "3")
	loader := New().
		Defaults(map[string]any{"server": map[string]any{"port": 1}}).
		File(path).
		Env("APP").
		Flags(map[string]any{"server": map[string]any{"port": 4}})

	if got := loader.Int("server.port", 0); got != 4 {
		t.Errorf("port = %d, esperado 4", got)
	}
	want := []string{"defaults", path, "env:APP", "flags"}
	if got := loader.Sources(); !reflect.DeepEqual(got, want) {
		t.Errorf("Sources = %#v, esperado %#v", got, want)
	}
}

// TestGetNotacaoDePonto cobre navegação, ausência e travessia de escalar.
func TestGetNotacaoDePonto(t *testing.T) {
	loader := New().Defaults(map[string]any{
		"a": map[string]any{
			"b": map[string]any{"c": 42},
			"d": nil,
		},
		"escalar": "texto",
	})

	if got, ok := loader.Get("a.b.c"); !ok || got != 42 {
		t.Errorf("Get(a.b.c) = (%v, %v), esperado (42, true)", got, ok)
	}
	if got, ok := loader.Get("a.d"); !ok || got != nil {
		t.Errorf("Get(a.d) = (%v, %v), esperado (nil, true)", got, ok)
	}
	if got, ok := loader.Get("a"); !ok || !reflect.DeepEqual(got, map[string]any{
		"b": map[string]any{"c": 42},
		"d": nil,
	}) {
		t.Errorf("Get(a) = (%#v, %v), esperado o mapa completo", got, ok)
	}
	if got, ok := loader.Get("a.b"); !ok || !reflect.DeepEqual(got, map[string]any{"c": 42}) {
		t.Errorf("Get(a.b) = (%#v, %v), esperado o mapa intermediário", got, ok)
	}
	for _, key := range []string{"", "a.b.c.d", "escalar.filho", "ausente"} {
		if got, ok := loader.Get(key); ok || got != nil {
			t.Errorf("Get(%q) = (%v, %v), esperado (nil, false)", key, got, ok)
		}
	}
}

// TestGetComDefaults cobre String, Int, Bool e Duration.
func TestGetComDefaults(t *testing.T) {
	loader := New().Defaults(map[string]any{
		"texto":           "valor",
		"numero":          123,
		"numeroTexto":     " 456 ",
		"numeroFloat":     7.0,
		"numeroQuebrado":  7.5,
		"json":            json.Number("89"),
		"numeroInvalido":  "abc",
		"booleano":        true,
		"boolTexto":       "yes",
		"boolNumero":      1,
		"boolInvalido":    "talvez",
		"duracao":         "1m30s",
		"duracaoNum":      int64(250 * time.Millisecond),
		"duracaoInvalida": "5",
	})

	if got := loader.String("texto", "def"); got != "valor" {
		t.Errorf("String = %q", got)
	}
	if got := loader.String("numero", "def"); got != "def" {
		t.Errorf("String de não-string = %q, esperado def", got)
	}
	if got := loader.String("ausente", "def"); got != "def" {
		t.Errorf("String ausente = %q", got)
	}
	for key, want := range map[string]int{
		"numero": 123, "numeroTexto": 456, "numeroFloat": 7, "json": 89,
	} {
		if got := loader.Int(key, 0); got != want {
			t.Errorf("Int(%q) = %d, esperado %d", key, got, want)
		}
	}
	for key, want := range map[string]int{"numeroQuebrado": -1, "numeroInvalido": -1, "ausente": -1} {
		if got := loader.Int(key, -1); got != want {
			t.Errorf("Int(%q) = %d, esperado default %d", key, got, want)
		}
	}
	for key, want := range map[string]bool{
		"booleano": true, "boolTexto": true, "boolNumero": true,
	} {
		if got := loader.Bool(key, false); got != want {
			t.Errorf("Bool(%q) = %v, esperado %v", key, got, want)
		}
	}
	for key := range map[string]bool{"boolInvalido": true, "ausente": true} {
		if got := loader.Bool(key, false); got {
			t.Errorf("Bool(%q) deveria usar o default false", key)
		}
	}
	if got := loader.Duration("duracao", 0); got != 90*time.Second {
		t.Errorf("Duration = %v, esperado 1m30s", got)
	}
	if got := loader.Duration("duracaoNum", 0); got != 250*time.Millisecond {
		t.Errorf("Duration numérica = %v, esperado 250ms", got)
	}
	if got := loader.Duration("duracaoInvalida", time.Second); got != time.Second {
		t.Errorf("Duration inválida = %v, esperado default 1s", got)
	}
	if got := loader.Duration("ausente", 2*time.Second); got != 2*time.Second {
		t.Errorf("Duration ausente = %v, esperado default 2s", got)
	}
}

// TestEnv cobre prefixo, conversão na leitura e chaves inválidas.
func TestEnv(t *testing.T) {
	t.Setenv("APP_SERVER_PORT", "8080")
	t.Setenv("APP_SERVER_TLS", "true")
	t.Setenv("APP_TIMEOUT", "1500ms")
	t.Setenv("APP_VAZIO", "")
	t.Setenv("APP_INVALIDO", "abc")
	t.Setenv("OUTRO_PORT", "9999")
	t.Setenv("APP_BAD__KEY", "ignorar")
	t.Setenv("APP_", "ignorar")

	loader := New().Env("app")
	if err := loader.Err(); err != nil {
		t.Fatalf("Env não deveria acumular erro: %v", err)
	}
	if got := loader.Int("server.port", 0); got != 8080 {
		t.Errorf("server.port = %d, esperado 8080", got)
	}
	if got := loader.Bool("server.tls", false); !got {
		t.Error("server.tls deveria ser true")
	}
	if got := loader.Duration("timeout", 0); got != 1500*time.Millisecond {
		t.Errorf("timeout = %v, esperado 1.5s", got)
	}
	if got := loader.String("vazio", "def"); got != "" {
		t.Errorf("vazio = %q, esperado string vazia", got)
	}
	if got := loader.Int("invalido", 7); got != 7 {
		t.Errorf("invalido = %d, esperado default 7", got)
	}
	if _, ok := loader.Get("outro.port"); ok {
		t.Error("variável sem o prefixo não deveria ser importada")
	}
	if _, ok := loader.Get("bad.key"); ok {
		t.Error("chave com segmento vazio não deveria ser importada")
	}
	if got, _ := loader.Get("server.port"); got != "8080" {
		t.Errorf("a camada deveria manter string, obteve %T", got)
	}
	if got := loader.Sources(); !reflect.DeepEqual(got, []string{"env:app"}) {
		t.Errorf("Sources = %#v", got)
	}
}

// TestEnvKey cobre a validação de nomes de variáveis diretamente.
func TestEnvKey(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		want   string
		ok     bool
	}{
		{"APP_PORT", "APP", "port", true},
		{"app_database_host", "APP", "database.host", true},
		{"APP", "APP", "", false},
		{"APP_", "APP", "", false},
		{"APP_A__B", "APP", "", false},
		{"APP_A.B", "APP", "", false},
		{"OTHER_PORT", "APP", "", false},
		{"PORT", "APP", "", false},
		{"PORT", "", "port", true},
		{"APP_PORT", "APP_", "", false},
	}
	for _, tt := range tests {
		got, ok := envKey(tt.prefix, tt.name)
		if got != tt.want || ok != tt.ok {
			t.Errorf("envKey(%q, %q) = (%q, %v), esperado (%q, %v)",
				tt.prefix, tt.name, got, ok, tt.want, tt.ok)
		}
	}
}

// TestFileJSONEYAML cobre leitura dos dois formatos.
func TestFileJSONEYAML(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(jsonPath, []byte(`{"server":{"port":8081},"hosts":["a","b"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	yamlPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(yamlPath, []byte("server:\n  host: yaml-host\nhosts:\n  - c\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	loader := New().File(jsonPath).File(yamlPath)
	if err := loader.Err(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got := loader.Int("server.port", 0); got != 8081 {
		t.Errorf("port = %d, esperado 8081", got)
	}
	if got := loader.String("server.host", ""); got != "yaml-host" {
		t.Errorf("host = %q, esperado yaml-host", got)
	}
	if got := loader.Int("hosts.0", 0); got != 0 {
		t.Errorf("listas não usam índices: obteve %d", got)
	}
	wantSources := []string{jsonPath, yamlPath}
	if got := loader.Sources(); !reflect.DeepEqual(got, wantSources) {
		t.Errorf("Sources = %#v, esperado %#v", got, wantSources)
	}

	// A lista da segunda camada substitui a primeira.
	hosts, ok := loader.Get("hosts")
	if !ok || !reflect.DeepEqual(hosts, []any{"c"}) {
		t.Errorf("hosts = %#v, esperado []any{\"c\"}", hosts)
	}
}

// TestFileAusenteNaoRegistraCamada cobre o contrato documentado para ausência.
func TestFileAusenteNaoRegistraCamada(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nao-existe.yaml")
	loader := New().File(path)
	if loader.Err() == nil {
		t.Fatal("Err deveria informar o arquivo ausente")
	}
	if !strings.Contains(loader.Err().Error(), "não existe") {
		t.Errorf("erro pouco claro: %v", loader.Err())
	}
	if got := loader.Sources(); len(got) != 0 {
		t.Errorf("camada ausente não deveria ser registrada: %#v", got)
	}
	if all := loader.All(); len(all) != 0 {
		t.Errorf("All deveria estar vazio: %#v", all)
	}
}

// TestFileInvalido cobre conteúdo inválido, topo não-objeto e extensão.
func TestFileInvalido(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		file    string
		content string
		want    string
	}{
		{"json inválido", "invalido.json", `{"server":`, "inválido"},
		{"yaml inválido", "invalido.yaml", "server: [sem-fechar\n", "inválido"},
		{"json vazio", "vazio.json", "  \n", "vazio"},
		{"yaml vazio", "vazio.yml", "", "vazio"},
		{"json topo lista", "lista.json", `[1,2]`, "objeto no topo"},
		{"yaml topo lista", "lista.yaml", "- a\n- b\n", "objeto no topo"},
		{"json extra", "extra.json", `{} {}`, "inválido"},
		{"yaml multi-doc", "multi.yaml", "a: 1\n---\nb: 2\n", "inválido"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.file)
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			loader := New().File(path)
			err := loader.Err()
			if err == nil {
				t.Fatal("Err deveria ser não nulo")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("erro = %q, esperado conter %q", err, tt.want)
			}
			if got := loader.Sources(); len(got) != 0 {
				t.Errorf("arquivo inválido não deveria ser registrado: %#v", got)
			}
		})
	}

	loader := New().File(filepath.Join(dir, "config.txt"))
	if loader.Err() == nil || !strings.Contains(loader.Err().Error(), "extensão") {
		t.Errorf("erro de extensão pouco claro: %v", loader.Err())
	}
}

// TestFileYAMLMaiusculo cobre a detecção sem diferenciar maiúsculas.
func TestFileYAMLMaiusculo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CONFIG.YML")
	if err := os.WriteFile(path, []byte("ok: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := New().File(path)
	if err := loader.Err(); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got := loader.Bool("ok", false); !got {
		t.Error("ok deveria ser true")
	}
}

// TestAllDeterministico cobre a estabilidade do resultado mesclado.
func TestAllDeterministico(t *testing.T) {
	loader := New().
		Defaults(map[string]any{"a": 1, "b": map[string]any{"c": 2}}).
		Flags(map[string]any{"b": map[string]any{"d": 3}})
	first := loader.All()
	for i := 0; i < 100; i++ {
		if got := loader.All(); !reflect.DeepEqual(got, first) {
			t.Fatalf("All divergiu na iteração %d: %#v != %#v", i, got, first)
		}
	}
}
