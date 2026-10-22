package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// tecla monta uma mensagem de tecla imprimível.
func tecla(text string) tea.KeyPressMsg {
	runes := []rune(text)
	if len(runes) == 0 {
		return tea.KeyPressMsg{}
	}
	return tea.KeyPressMsg{Text: text, Code: runes[0]}
}

// teclaEspecial monta uma mensagem para uma tecla de controle.
func teclaEspecial(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// atualizarEditor aplica uma mensagem e devolve o editor resultante.
func atualizarEditor(t *testing.T, editor Editor, msg tea.Msg) Editor {
	t.Helper()
	model, _ := editor.Update(msg)
	updated, ok := model.(Editor)
	if !ok {
		t.Fatalf("Update devolveu %T, esperado Editor", model)
	}
	return updated
}

// textoDoEditor devolve a View sem códigos ANSI.
func textoDoEditor(editor Editor) string {
	return ansi.Strip(editor.View().Content)
}

// TestEditorImplementaTeaModel garante a compatibilidade com Bubble Tea.
func TestEditorImplementaTeaModel(t *testing.T) {
	var _ tea.Model = Editor{}
	var _ tea.Model = (*Editor)(nil)
}

// editorDeTeste cria um editor com três campos simples.
func editorDeTeste(t *testing.T) Editor {
	t.Helper()
	loader := New().Defaults(map[string]any{
		"a": 1,
		"b": 2,
		"c": 3,
	})
	schema := Schema{Fields: []Field{
		{Key: "a", Type: "int", Doc: "Campo A"},
		{Key: "b", Type: "int", Doc: "Campo B"},
		{Key: "c", Type: "int", Doc: "Campo C"},
	}}
	editor := NewEditor(loader, schema)
	editor.SetSize(80, 10)
	return editor
}

// TestEditorNavegacao cobre setas, limites e a tecla tab.
func TestEditorNavegacao(t *testing.T) {
	editor := editorDeTeste(t)
	if view := textoDoEditor(editor); !strings.Contains(view, "> a = 1") {
		t.Fatalf("seleção inicial ausente:\n%s", view)
	}

	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyDown))
	if view := textoDoEditor(editor); !strings.Contains(view, "> b = 2") {
		t.Fatalf("down não moveu a seleção:\n%s", view)
	}
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyDown))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyDown))
	if view := textoDoEditor(editor); !strings.Contains(view, "> c = 3") {
		t.Fatalf("down deveria parar no último campo:\n%s", view)
	}
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyUp))
	if view := textoDoEditor(editor); !strings.Contains(view, "> b = 2") {
		t.Fatalf("up não moveu a seleção:\n%s", view)
	}

	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyTab))
	view := textoDoEditor(editor)
	if !strings.Contains(view, "[docs]") || !strings.Contains(view, "Campo B") {
		t.Fatalf("tab deveria abrir a documentação:\n%s", view)
	}
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyTab))
	if view := textoDoEditor(editor); !strings.Contains(view, "[valores]") {
		t.Fatalf("tab deveria voltar aos valores:\n%s", view)
	}
}

// TestEditorEdicaoInline cobre enter, edição, commit, esc e dirty.
func TestEditorEdicaoInline(t *testing.T) {
	loader := New().Defaults(map[string]any{
		"server":  map[string]any{"port": 8080},
		"timeout": "1m",
		"tls":     false,
		"hosts":   []string{"a"},
	})
	schema := Schema{Fields: []Field{
		{Key: "server.port", Type: "int"},
		{Key: "timeout", Type: "duration"},
		{Key: "tls", Type: "bool"},
		{Key: "hosts", Type: "list"},
	}}
	editor := NewEditor(loader, schema)
	editor.SetSize(80, 12)

	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if view := textoDoEditor(editor); !strings.Contains(view, "8080█") {
		t.Fatalf("edição inline não exibiu o cursor:\n%s", view)
	}
	for i := 0; i < 4; i++ {
		editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyBackspace))
	}
	editor = atualizarEditor(t, editor, tecla("9090"))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if !editor.Dirty() {
		t.Fatal("editor deveria estar dirty após a edição")
	}
	if got, ok := lookup(editor.Values(), "server.port"); !ok || got != 9090 {
		t.Errorf("server.port = %#v, esperado 9090", got)
	}

	// Editar e cancelar não altera o valor.
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	editor = atualizarEditor(t, editor, tecla("1"))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEscape))
	if got, _ := lookup(editor.Values(), "server.port"); got != 9090 {
		t.Errorf("esc não deveria alterar o valor: %#v", got)
	}
	if editor.state.editing {
		t.Error("esc deveria encerrar a edição")
	}

	// Duração, bool e lista são convertidos no commit.
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyDown))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	for i := 0; i < len("1m0s"); i++ {
		editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyBackspace))
	}
	editor = atualizarEditor(t, editor, tecla("1m30s"))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if got, _ := lookup(editor.Values(), "timeout"); got != 90*time.Second {
		t.Errorf("timeout = %#v, esperado 1m30s", got)
	}

	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyDown))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	for i := 0; i < len("false"); i++ {
		editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyBackspace))
	}
	editor = atualizarEditor(t, editor, tecla("true"))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if got, _ := lookup(editor.Values(), "tls"); got != true {
		t.Errorf("tls = %#v, esperado true", got)
	}

	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyDown))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyBackspace))
	editor = atualizarEditor(t, editor, tecla("x, y"))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if got, _ := lookup(editor.Values(), "hosts"); !reflect.DeepEqual(got, []string{"x", "y"}) {
		t.Errorf("hosts = %#v, esperado []string{\"x\", \"y\"}", got)
	}
}

// TestEditorEdicaoInvalida garante que valores inválidos não são aplicados.
func TestEditorEdicaoInvalida(t *testing.T) {
	editor := NewEditor(New(), Schema{Fields: []Field{{Key: "port", Type: "int"}}})
	editor.SetSize(80, 8)

	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	editor = atualizarEditor(t, editor, tecla("abc"))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if !editor.state.editing {
		t.Fatal("edição inválida deveria continuar aberta")
	}
	if !editor.state.statusErr {
		t.Error("status deveria indicar erro")
	}
	if editor.Dirty() {
		t.Error("valor inválido não deveria marcar dirty")
	}
	if view := textoDoEditor(editor); !strings.Contains(view, "inválido") {
		t.Fatalf("erro não apareceu na View:\n%s", view)
	}
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEscape))
	if editor.state.editing || editor.state.statusErr {
		t.Error("esc deveria limpar a edição e o erro")
	}
}

// TestEditorEdicaoSemMudancaNaoSuja cobre o commit do mesmo valor.
func TestEditorEdicaoSemMudancaNaoSuja(t *testing.T) {
	editor := NewEditor(New().Defaults(map[string]any{"port": 8080}),
		Schema{Fields: []Field{{Key: "port", Type: "int"}}})
	editor.SetSize(80, 8)
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if editor.Dirty() {
		t.Error("commitar o mesmo valor não deveria marcar dirty")
	}
}

// TestEditorDefaultPreenche cobre o uso de Field.Default no editor.
func TestEditorDefaultPreenche(t *testing.T) {
	editor := NewEditor(New(), Schema{Fields: []Field{
		{Key: "port", Type: "int", Default: 8080},
	}})
	editor.SetSize(80, 8)
	if got, ok := lookup(editor.Values(), "port"); !ok || got != 8080 {
		t.Errorf("port = %#v, esperado default 8080", got)
	}
	if editor.Dirty() {
		t.Error("defaults iniciais não deveriam marcar dirty")
	}
}

// TestEditorNormalizaEnv cobre a normalização de valores de ambiente.
func TestEditorNormalizaEnv(t *testing.T) {
	t.Setenv("APP_PORT", "8080")
	loader := New().Env("APP")
	editor := NewEditor(loader, Schema{Fields: []Field{{Key: "port", Type: "int"}}})
	editor.SetSize(80, 8)
	if got, ok := lookup(editor.Values(), "port"); !ok || got != 8080 {
		t.Errorf("port = %#v, esperado int 8080", got)
	}
}

// TestEditorWidth cobre larguras e alturas pequenas, zero e negativas.
func TestEditorWidth(t *testing.T) {
	loader := New().Defaults(map[string]any{
		"chave.muito.longa": "um valor bastante comprido para truncar",
	})
	schema := Schema{Fields: []Field{
		{Key: "chave.muito.longa", Type: "string", Doc: strings.Repeat("documentação longa ", 10)},
	}}
	cases := []struct {
		width  int
		height int
		empty  bool
	}{
		{0, 10, true},
		{80, 0, true},
		{-1, 10, true},
		{80, -1, true},
		{20, 10, false},
		{80, 5, false},
		{1, 1, false},
	}
	for _, tt := range cases {
		editor := NewEditor(loader, schema)
		editor.SetSize(tt.width, tt.height)
		view := editor.View().Content
		if tt.empty {
			if view != "" {
				t.Errorf("SetSize(%d,%d) deveria devolver View vazia: %q", tt.width, tt.height, view)
			}
			continue
		}
		lines := strings.Split(view, "\n")
		if len(lines) > tt.height {
			t.Errorf("SetSize(%d,%d): %d linhas ultrapassam a altura", tt.width, tt.height, len(lines))
		}
		for i, line := range lines {
			if got := lipgloss.Width(line); got > tt.width {
				t.Errorf("SetSize(%d,%d): linha %d tem largura %d: %q",
					tt.width, tt.height, i, got, ansi.Strip(line))
			}
		}
	}
}

// TestEditorWindowSize cobre tea.WindowSizeMsg.
func TestEditorWindowSize(t *testing.T) {
	editor := editorDeTeste(t)
	editor = atualizarEditor(t, editor, tea.WindowSizeMsg{Width: 20, Height: 3})
	view := editor.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) != 3 {
		t.Errorf("esperadas 3 linhas, obteve %d: %q", len(lines), ansi.Strip(view))
	}
	for _, line := range lines {
		if lipgloss.Width(line) > 20 {
			t.Errorf("linha excede a largura: %q", ansi.Strip(line))
		}
	}
	editor = atualizarEditor(t, editor, tea.WindowSizeMsg{Width: 0, Height: 3})
	if editor.View().Content != "" {
		t.Error("WindowSizeMsg com largura zero deveria esvaziar a View")
	}
}

// TestEditorDeterminismo garante 100 renders idênticos.
func TestEditorDeterminismo(t *testing.T) {
	loader := New().Defaults(map[string]any{
		"a":    "texto",
		"b":    2,
		"meta": map[string]any{"z": 1, "a": 2},
	})
	schema := Schema{Fields: []Field{
		{Key: "a", Type: "string", Doc: "Campo A"},
		{Key: "b", Type: "int", Doc: "Campo B"},
		{Key: "meta", Type: "string"},
	}}
	editor := NewEditor(loader, schema)
	editor.SetSize(64, 10)
	first := editor.View().Content
	for i := 0; i < 100; i++ {
		if got := editor.View().Content; got != first {
			t.Fatalf("View divergiu na iteração %d:\n%q\n%q", i, got, first)
		}
	}
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyTab))
	docs := editor.View().Content
	for i := 0; i < 100; i++ {
		if got := editor.View().Content; got != docs {
			t.Fatalf("View de docs divergiu na iteração %d", i)
		}
	}
}

// TestEditorSaveYAML cobre gravação, roundtrip, ordenação e permissões.
func TestEditorSaveYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	initial := "server:\n  port: 8080\nname: app\n"
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := New().File(path)
	if err := loader.Err(); err != nil {
		t.Fatal(err)
	}
	editor := NewEditor(loader, Schema{Fields: []Field{
		{Key: "server.port", Type: "int"},
		{Key: "name", Type: "string"},
	}})
	editor.SetSize(80, 10)

	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	for i := 0; i < 4; i++ {
		editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyBackspace))
	}
	editor = atualizarEditor(t, editor, tecla("9090"))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if !editor.Dirty() {
		t.Fatal("editor deveria estar dirty")
	}
	if err := editor.Save(); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}
	if editor.Dirty() {
		t.Error("Save deveria limpar dirty")
	}
	if view := textoDoEditor(editor); !strings.Contains(view, "salvo em") {
		t.Errorf("status de save ausente:\n%s", view)
	}

	round := New().File(path)
	if err := round.Err(); err != nil {
		t.Fatalf("arquivo gravado é inválido: %v", err)
	}
	if got := round.Int("server.port", 0); got != 9090 {
		t.Errorf("roundtrip port = %d, esperado 9090", got)
	}
	if got := round.String("name", ""); got != "app" {
		t.Errorf("roundtrip name = %q, esperado app", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Index(content, "name:") > strings.Index(content, "server:") {
		t.Errorf("chaves não estão ordenadas:\n%s", content)
	}
	if !strings.Contains(content, "server:\n    port: 9090") {
		t.Errorf("YAML sem o valor esperado:\n%s", content)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("permissões = %o, esperado 600", got)
		}
	}
}

// TestEditorSaveJSON cobre a manutenção do formato JSON.
func TestEditorSaveJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"server":{"port":8080}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := New().File(path)
	if err := loader.Err(); err != nil {
		t.Fatal(err)
	}
	editor := NewEditor(loader, Schema{Fields: []Field{{Key: "server.port", Type: "int"}}})
	editor.SetSize(80, 8)
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	for i := 0; i < 4; i++ {
		editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyBackspace))
	}
	editor = atualizarEditor(t, editor, tecla("9090"))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if err := editor.Save(); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "{") || !strings.Contains(content, `"port": 9090`) {
		t.Errorf("JSON inesperado:\n%s", content)
	}
	round := New().File(path)
	if got := round.Int("server.port", 0); got != 9090 {
		t.Errorf("roundtrip port = %d, esperado 9090", got)
	}
}

// TestEditorSaveSemArquivo cobre a ausência de camada gravável.
func TestEditorSaveSemArquivo(t *testing.T) {
	editor := NewEditor(New().Defaults(map[string]any{"port": 8080}),
		Schema{Fields: []Field{{Key: "port", Type: "int"}}})
	editor.SetSize(80, 8)
	err := editor.Save()
	if !errors.Is(err, ErrNoWritableFile) {
		t.Fatalf("erro = %v, esperado ErrNoWritableFile", err)
	}
	if !editor.state.statusErr {
		t.Error("status deveria indicar erro")
	}
	if view := textoDoEditor(editor); !strings.Contains(view, "nenhum arquivo") {
		t.Errorf("erro não apareceu na View:\n%s", view)
	}

	var zero Editor
	if err := zero.Save(); !errors.Is(err, ErrNoWritableFile) {
		t.Errorf("Editor zero: erro = %v", err)
	}
}

// TestEditorSavePrioridade cobre a escolha do arquivo de maior prioridade.
func TestEditorSavePrioridade(t *testing.T) {
	dir := t.TempDir()
	low := filepath.Join(dir, "base.yaml")
	high := filepath.Join(dir, "override.yaml")
	if err := os.WriteFile(low, []byte("port: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(high, []byte("port: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := New().
		Add(Source{Name: "baixa", Priority: 10, Data: map[string]any{"port": 1}, path: low, format: formatYAML}).
		Add(Source{Name: "alta", Priority: 20, Data: map[string]any{"port": 2}, path: high, format: formatYAML})
	editor := NewEditor(loader, Schema{Fields: []Field{{Key: "port", Type: "int"}}})
	editor.SetSize(80, 8)
	if err := editor.Save(); err != nil {
		t.Fatalf("Save falhou: %v", err)
	}
	lowData, _ := os.ReadFile(low)
	highData, _ := os.ReadFile(high)
	if !strings.Contains(string(lowData), "port: 1") {
		t.Errorf("arquivo de menor prioridade foi alterado:\n%s", lowData)
	}
	if !strings.Contains(string(highData), "port: 2") {
		t.Errorf("arquivo de maior prioridade não recebeu a gravação:\n%s", highData)
	}
}

// TestEditorQuitConfirmacao cobre a saída com e sem alterações.
func TestEditorQuitConfirmacao(t *testing.T) {
	editor := NewEditor(New().Defaults(map[string]any{"port": 8080}),
		Schema{Fields: []Field{{Key: "port", Type: "int"}}})
	editor.SetSize(80, 8)
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	editor = atualizarEditor(t, editor, tecla("1"))
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if !editor.Dirty() {
		t.Fatal("editor deveria estar dirty")
	}

	model, cmd := editor.Update(tecla("q"))
	editor = model.(Editor)
	if cmd != nil {
		t.Fatal("o primeiro q não deveria encerrar")
	}
	if !editor.state.confirmQuit {
		t.Fatal("o primeiro q deveria pedir confirmação")
	}
	if view := textoDoEditor(editor); !strings.Contains(view, "não salvas") {
		t.Fatalf("confirmação ausente:\n%s", view)
	}

	editor = atualizarEditor(t, editor, tecla("n"))
	if editor.state.confirmQuit {
		t.Fatal("n deveria cancelar a confirmação")
	}
	editor = atualizarEditor(t, editor, tecla("q"))
	if !editor.state.confirmQuit {
		t.Fatal("q deveria pedir confirmação novamente")
	}
	_, cmd = editor.Update(tecla("q"))
	if cmd == nil {
		t.Fatal("o segundo q deveria encerrar")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("comando = %T, esperado tea.QuitMsg", cmd())
	}

	clean := editorDeTeste(t)
	_, cmd = clean.Update(tecla("q"))
	if cmd == nil {
		t.Fatal("q sem alterações deveria encerrar imediatamente")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("comando = %T, esperado tea.QuitMsg", cmd())
	}
}

// TestEditorValuesCopia garante que Values não compartilha o estado interno.
func TestEditorValuesCopia(t *testing.T) {
	editor := NewEditor(New().Defaults(map[string]any{
		"server": map[string]any{"port": 8080},
	}), Schema{Fields: []Field{{Key: "server.port", Type: "int"}}})
	values := editor.Values()
	server := values["server"].(map[string]any)
	server["port"] = 1
	if got, _ := lookup(editor.Values(), "server.port"); got != 8080 {
		t.Errorf("Values devolveu mapa compartilhado: %#v", got)
	}
}

// TestEditorSchemaVazio cobre schema sem campos e dimensões pequenas.
func TestEditorSchemaVazio(t *testing.T) {
	editor := NewEditor(New(), Schema{})
	editor.SetSize(20, 5)
	view := textoDoEditor(editor)
	if !strings.Contains(view, "(nenhum campo)") {
		t.Fatalf("estado vazio não indicado:\n%s", view)
	}
	editor = atualizarEditor(t, editor, teclaEspecial(tea.KeyEnter))
	if editor.state.editing {
		t.Error("enter sem campos não deveria abrir edição")
	}
}
