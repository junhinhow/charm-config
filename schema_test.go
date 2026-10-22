package config

import (
	"strings"
	"testing"
	"time"
)

// TestSchemaValidateQuatroErros garante que todos os erros são devolvidos.
func TestSchemaValidateQuatroErros(t *testing.T) {
	schema := Schema{Fields: []Field{
		{Key: "nome", Type: "string", Required: true},
		{Key: "server.port", Type: "int"},
		{Key: "server.tls", Type: "bool"},
		{Key: "timeout", Type: "duration"},
	}}
	all := map[string]any{
		"server": map[string]any{
			"port": "abc",
			"tls":  "talvez",
		},
		"timeout": "logo",
	}

	errs := schema.Validate(all)
	if len(errs) != 4 {
		t.Fatalf("esperados 4 erros, obteve %d: %v", len(errs), errs)
	}
	wants := []string{
		`campo "nome" é obrigatório`,
		`campo "server.port": esperado int`,
		`campo "server.tls": esperado bool`,
		`campo "timeout": esperado duration`,
	}
	for i, want := range wants {
		if !strings.Contains(errs[i].Error(), want) {
			t.Errorf("erro %d = %q, esperado conter %q", i, errs[i], want)
		}
	}
}

// TestSchemaValidateSucesso cobre todos os tipos, inclusive strings de env.
func TestSchemaValidateSucesso(t *testing.T) {
	schema := Schema{Fields: []Field{
		{Key: "nome", Type: "string", Required: true},
		{Key: "server.port", Type: "int", Required: true},
		{Key: "server.tls", Type: "bool"},
		{Key: "timeout", Type: "duration"},
		{Key: "hosts", Type: "list"},
	}}
	all := map[string]any{
		"nome": "app",
		"server": map[string]any{
			"port": "8080",
			"tls":  "yes",
		},
		"timeout": "1m30s",
		"hosts":   []string{"a", "b"},
	}

	if errs := schema.Validate(all); len(errs) != 0 {
		t.Fatalf("erros inesperados: %v", errs)
	}

	typed := map[string]any{
		"nome":    "app",
		"server":  map[string]any{"port": 8080, "tls": true},
		"timeout": 90 * time.Second,
		"hosts":   []any{"a"},
	}
	if errs := schema.Validate(typed); len(errs) != 0 {
		t.Fatalf("erros inesperados com tipos nativos: %v", errs)
	}
}

// TestSchemaDefaultNaoPreenche garante que Default é apenas do editor.
func TestSchemaDefaultNaoPreenche(t *testing.T) {
	schema := Schema{Fields: []Field{
		{Key: "port", Type: "int", Required: true, Default: 8080},
	}}
	all := map[string]any{}
	errs := schema.Validate(all)
	if len(errs) != 1 {
		t.Fatalf("esperado 1 erro, obteve %d: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), "obrigatório") {
		t.Errorf("erro = %q, esperado obrigatório", errs[0])
	}
	if _, ok := all["port"]; ok {
		t.Error("Validate não deveria alterar o mapa")
	}
}

// TestSchemaTipoDesconhecido cobre erros de configuração do schema.
func TestSchemaTipoDesconhecido(t *testing.T) {
	schema := Schema{Fields: []Field{{Key: "x", Type: "complex"}}}
	errs := schema.Validate(map[string]any{"x": 1})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "tipo desconhecido") {
		t.Fatalf("erro = %v, esperado tipo desconhecido", errs)
	}
}

// TestSchemaTipoErrado cobre nil, lista e escalares incompatíveis.
func TestSchemaTipoErrado(t *testing.T) {
	schema := Schema{Fields: []Field{
		{Key: "texto", Type: "string"},
		{Key: "numero", Type: "int"},
		{Key: "lista", Type: "list"},
		{Key: "duracao", Type: "duration"},
	}}
	all := map[string]any{
		"texto":   1,
		"numero":  "12.5",
		"lista":   "a,b",
		"duracao": 12.5,
	}
	errs := schema.Validate(all)
	if len(errs) != 4 {
		t.Fatalf("esperados 4 erros, obteve %d: %v", len(errs), errs)
	}
	for _, err := range errs {
		if !strings.Contains(err.Error(), "esperado") {
			t.Errorf("mensagem sem o tipo esperado: %q", err)
		}
	}

	nilSchema := Schema{Fields: []Field{{Key: "x", Type: "bool"}}}
	errs = nilSchema.Validate(map[string]any{"x": nil})
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "nil") {
		t.Fatalf("erro = %v, esperado menção a nil", errs)
	}
}

// TestSchemaCamposOpcionaisAusentes garante que campos opcionais são ignorados.
func TestSchemaCamposOpcionaisAusentes(t *testing.T) {
	schema := Schema{Fields: []Field{
		{Key: "a", Type: "string"},
		{Key: "b", Type: "int"},
	}}
	if errs := schema.Validate(nil); len(errs) != 0 {
		t.Fatalf("erros inesperados: %v", errs)
	}
}
