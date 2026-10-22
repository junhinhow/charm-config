package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"go.yaml.in/yaml/v3"
)

// ErrNoWritableFile indica que [Editor.Save] não encontrou nenhuma camada de
// arquivo para receber a configuração.
var ErrNoWritableFile = errors.New(
	"config: nenhum arquivo gravável; adicione uma camada com Loader.File")

// Estilos usados pelo editor.
var (
	editorTitleStyle    = lipgloss.NewStyle().Bold(true)
	editorSelectedStyle = lipgloss.NewStyle().Reverse(true)
	editorFaintStyle    = lipgloss.NewStyle().Faint(true)
	editorAlertStyle    = lipgloss.NewStyle().Bold(true)
)

// editorMode alterna entre a lista de valores e a lista de documentação.
type editorMode int

const (
	// editorValues mostra as chaves e os valores.
	editorValues editorMode = iota
	// editorDocs mostra as chaves e a documentação.
	editorDocs
)

// editorState concentra o estado mutável compartilhado pelas cópias de
// [Editor].
type editorState struct {
	loader      *Loader
	schema      Schema
	values      map[string]any
	initial     map[string]any
	cursor      int
	mode        editorMode
	editing     bool
	buffer      []rune
	position    int
	width       int
	height      int
	status      string
	statusErr   bool
	confirmQuit bool
}

// Editor é o modelo Bubble Tea para editar a configuração.
//
// Cópias de Editor compartilham o mesmo estado interno. Isso permite que
// [Editor.Update], que tem receptor por valor para satisfazer [tea.Model],
// altere o estado observado pelo ponteiro devolvido por [NewEditor]. O valor
// zero é utilizável depois da primeira chamada a um método de mutação.
//
// O editor trabalha sobre uma cópia da configuração mesclada no momento da
// criação. Alterações posteriores no [Loader] não são refletidas até que um
// novo editor seja criado.
type Editor struct {
	state *editorState
}

// NewEditor cria um editor para o loader e o schema informados.
//
// Valores ausentes que tenham [Field.Default] começam preenchidos no editor.
// Valores convertíveis são normalizados para o tipo do campo, de modo que
// [Editor.Values] devolve int, bool e time.Duration quando possível. O schema
// é copiado; alterações posteriores na fatia original não afetam o editor.
func NewEditor(loader *Loader, schema Schema) Editor {
	if loader == nil {
		loader = New()
	}
	schema.Fields = append([]Field(nil), schema.Fields...)
	values := loader.All()
	for _, field := range schema.Fields {
		value, ok := lookup(values, field.Key)
		if !ok {
			if field.Default == nil {
				continue
			}
			value = cloneValue(field.Default)
			ok = true
			setPath(values, field.Key, value)
		}
		if normalized, ok := normalizeFieldValue(field, value); ok {
			setPath(values, field.Key, normalized)
		}
	}
	return Editor{state: &editorState{
		loader:  loader,
		schema:  schema,
		values:  values,
		initial: cloneMap(values),
		width:   80,
		height:  24,
	}}
}

// newEditorState cria um estado vazio com tamanho inicial de 80x24.
func newEditorState() *editorState {
	return &editorState{
		loader:  New(),
		values:  make(map[string]any),
		initial: make(map[string]any),
		width:   80,
		height:  24,
	}
}

// ensureState inicializa o estado do valor zero, se necessário.
func (e *Editor) ensureState() *editorState {
	if e.state == nil {
		e.state = newEditorState()
	}
	return e.state
}

// SetSize define a largura e a altura da área de desenho.
//
// Valores negativos são tratados como zero. Com largura ou altura zero,
// [Editor.View] devolve conteúdo vazio.
func (e *Editor) SetSize(width, height int) {
	e.ensureState().setSize(width, height)
}

// setSize aplica as dimensões com validação de valores negativos.
func (st *editorState) setSize(width, height int) {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	st.width = width
	st.height = height
}

// Values devolve uma cópia dos valores atuais do editor.
//
// O mapa é aninhado, como em [Loader.All], e inclui chaves fora do schema
// preservadas da configuração original. Alterar o resultado não modifica o
// editor.
func (e Editor) Values() map[string]any {
	if e.state == nil || e.state.values == nil {
		return make(map[string]any)
	}
	return cloneMap(e.state.values)
}

// Dirty informa se há alterações ainda não salvas.
func (e Editor) Dirty() bool {
	if e.state == nil {
		return false
	}
	return e.state.dirty()
}

// dirty informa se os valores divergem do último estado salvo.
func (st *editorState) dirty() bool {
	return !reflect.DeepEqual(st.values, st.initial)
}

// Save grava a configuração no arquivo de maior prioridade entre as camadas de
// arquivo, em YAML ou JSON conforme o formato original.
//
// O arquivo é gravado com permissão 0600 e chaves ordenadas. O conteúdo é a
// configuração efetiva do editor, incluindo valores vindos de outras camadas;
// por isso, salvar materializa essas camadas no arquivo escolhido. Se não
// houver camada de arquivo, devolve [ErrNoWritableFile].
func (e *Editor) Save() error {
	return e.ensureState().save()
}

// save implementa a gravação e atualiza o estado do editor.
func (st *editorState) save() error {
	source, ok := st.writableSource()
	if !ok {
		return st.fail(ErrNoWritableFile)
	}
	data, err := encodeConfig(st.saveValues(), source.format)
	if err != nil {
		return st.fail(err)
	}
	if err := os.WriteFile(source.path, data, 0o600); err != nil {
		return st.fail(fmt.Errorf("config: não foi possível gravar %q: %w", source.path, err))
	}
	if err := os.Chmod(source.path, 0o600); err != nil {
		return st.fail(fmt.Errorf(
			"config: não foi possível ajustar as permissões de %q para 0600: %w",
			source.path, err))
	}
	st.initial = cloneMap(st.values)
	st.status = "salvo em " + source.path
	st.statusErr = false
	return nil
}

// fail registra e devolve um erro de gravação.
func (st *editorState) fail(err error) error {
	st.status = err.Error()
	st.statusErr = true
	return err
}

// writableSource escolhe a camada de arquivo de maior prioridade.
func (st *editorState) writableSource() (Source, bool) {
	if st.loader == nil {
		return Source{}, false
	}
	var chosen Source
	found := false
	for _, source := range st.loader.orderedSources() {
		if source.path != "" && source.format != formatNone {
			chosen = source
			found = true
		}
	}
	return chosen, found
}

// saveValues devolve uma cópia normalizada dos valores para serialização.
func (st *editorState) saveValues() map[string]any {
	out := cloneMap(st.values)
	if out == nil {
		out = make(map[string]any)
	}
	for _, field := range st.schema.Fields {
		value, ok := lookup(out, field.Key)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(field.Type)) {
		case "int":
			if n, ok := toInt(value); ok {
				setPath(out, field.Key, n)
			}
		case "bool":
			if b, ok := toBool(value); ok {
				setPath(out, field.Key, b)
			}
		case "duration":
			if d, ok := toDuration(value); ok {
				setPath(out, field.Key, d.String())
			}
		case "string":
			if s, ok := value.(string); ok {
				setPath(out, field.Key, s)
			}
		case "list":
			if isList(value) {
				setPath(out, field.Key, cloneValue(value))
			}
		}
	}
	return out
}

// encodeConfig serializa os valores no formato indicado.
func encodeConfig(values map[string]any, format fileFormat) ([]byte, error) {
	switch format {
	case formatJSON:
		data, err := json.MarshalIndent(values, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("config: não foi possível serializar JSON: %w", err)
		}
		return append(data, '\n'), nil
	case formatYAML:
		data, err := yaml.Marshal(values)
		if err != nil {
			return nil, fmt.Errorf("config: não foi possível serializar YAML: %w", err)
		}
		return data, nil
	default:
		return nil, errors.New("config: formato de arquivo desconhecido")
	}
}

// Init implementa [tea.Model] e não executa nenhum comando inicial.
func (e Editor) Init() tea.Cmd {
	return nil
}

// Update implementa [tea.Model].
//
// WindowSizeMsg atualiza as dimensões. As teclas de navegação e edição são
// descritas na documentação do pacote e no README.
func (e Editor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if e.state == nil {
		e.state = newEditorState()
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		e.state.setSize(msg.Width, msg.Height)
		return e, nil
	case tea.KeyPressMsg:
		return e.handleKey(msg)
	default:
		return e, nil
	}
}

// View implementa [tea.Model] e devolve a tela do editor.
//
// Nenhuma linha ultrapassa a largura definida por [Editor.SetSize] e a saída
// nunca tem mais linhas que a altura definida. Com largura ou altura menor ou
// igual a zero, o conteúdo é vazio.
func (e Editor) View() tea.View {
	if e.state == nil || e.state.width <= 0 || e.state.height <= 0 {
		return tea.View{}
	}
	return tea.NewView(e.state.render())
}

// handleKey processa uma tecla de acordo com o modo atual.
func (e Editor) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	st := e.state
	key := msg.String()

	if st.confirmQuit {
		switch key {
		case "q", "y", "enter", "ctrl+c":
			return e, tea.Quit
		case "n", "esc":
			st.confirmQuit = false
			st.status = ""
			st.statusErr = false
		}
		return e, nil
	}

	if st.editing {
		st.handleEditingKey(msg)
		return e, nil
	}

	switch key {
	case "up", "k":
		st.move(-1)
	case "down", "j":
		st.move(1)
	case "tab":
		if st.mode == editorValues {
			st.mode = editorDocs
		} else {
			st.mode = editorValues
		}
	case "enter":
		st.startEdit()
	case "s":
		_ = st.save()
	case "q", "ctrl+c":
		if st.dirty() {
			st.confirmQuit = true
			st.status = ""
			st.statusErr = false
			return e, nil
		}
		return e, tea.Quit
	}
	return e, nil
}

// handleEditingKey processa teclas durante a edição inline.
func (st *editorState) handleEditingKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		st.cancelEdit()
		return
	case "enter":
		st.commitEdit()
		return
	case "backspace":
		if st.position > 0 {
			st.buffer = append(st.buffer[:st.position-1], st.buffer[st.position:]...)
			st.position--
		}
		return
	case "delete":
		if st.position < len(st.buffer) {
			st.buffer = append(st.buffer[:st.position], st.buffer[st.position+1:]...)
		}
		return
	case "left":
		if st.position > 0 {
			st.position--
		}
		return
	case "right":
		if st.position < len(st.buffer) {
			st.position++
		}
		return
	case "home":
		st.position = 0
		return
	case "end":
		st.position = len(st.buffer)
		return
	case "tab":
		return
	}

	for _, r := range msg.Text {
		if !unicode.IsPrint(r) {
			continue
		}
		st.buffer = append(st.buffer, 0)
		copy(st.buffer[st.position+1:], st.buffer[st.position:])
		st.buffer[st.position] = r
		st.position++
	}
}

// move desloca o cursor sem dar a volta.
func (st *editorState) move(delta int) {
	total := len(st.schema.Fields)
	if total == 0 {
		st.cursor = 0
		return
	}
	st.cursor = st.index() + delta
	if st.cursor < 0 {
		st.cursor = 0
	}
	if st.cursor >= total {
		st.cursor = total - 1
	}
}

// index devolve o índice do cursor limitado aos campos existentes.
func (st *editorState) index() int {
	total := len(st.schema.Fields)
	if total == 0 {
		return 0
	}
	if st.cursor < 0 {
		return 0
	}
	if st.cursor >= total {
		return total - 1
	}
	return st.cursor
}

// currentField devolve o campo selecionado.
func (st *editorState) currentField() (Field, bool) {
	if len(st.schema.Fields) == 0 {
		return Field{}, false
	}
	return st.schema.Fields[st.index()], true
}

// startEdit inicia a edição inline do campo selecionado.
func (st *editorState) startEdit() {
	field, ok := st.currentField()
	if !ok {
		return
	}
	st.cursor = st.index()
	value, exists := lookup(st.values, field.Key)
	if !exists && field.Default != nil {
		value = field.Default
		exists = true
	}
	st.buffer = []rune(editValue(field, value, exists))
	st.position = len(st.buffer)
	st.editing = true
	st.status = ""
	st.statusErr = false
}

// cancelEdit descarta a edição em andamento.
func (st *editorState) cancelEdit() {
	st.editing = false
	st.buffer = nil
	st.position = 0
	st.status = ""
	st.statusErr = false
}

// commitEdit valida e aplica o texto editado.
func (st *editorState) commitEdit() {
	field, ok := st.currentField()
	if !ok {
		st.cancelEdit()
		return
	}
	value, err := parseFieldValue(field, string(st.buffer))
	if err != nil {
		st.status = err.Error()
		st.statusErr = true
		return
	}
	setPath(st.values, field.Key, value)
	st.editing = false
	st.buffer = nil
	st.position = 0
	st.status = ""
	st.statusErr = false
}

// normalizeFieldValue converte um valor para o tipo do campo, quando possível.
func normalizeFieldValue(field Field, value any) (any, bool) {
	switch strings.ToLower(strings.TrimSpace(field.Type)) {
	case "string":
		s, ok := value.(string)
		return s, ok
	case "int":
		n, ok := toInt(value)
		return n, ok
	case "bool":
		b, ok := toBool(value)
		return b, ok
	case "duration":
		d, ok := toDuration(value)
		return d, ok
	case "list":
		if !isList(value) {
			return nil, false
		}
		return cloneValue(value), true
	default:
		return value, true
	}
}

// parseFieldValue interpreta o texto digitado conforme o tipo do campo.
func parseFieldValue(field Field, text string) (any, error) {
	switch strings.ToLower(strings.TrimSpace(field.Type)) {
	case "string":
		return text, nil
	case "int":
		n, ok := toInt(strings.TrimSpace(text))
		if !ok {
			return nil, fmt.Errorf("campo %q: valor inteiro inválido %q", field.Key, text)
		}
		return n, nil
	case "bool":
		b, ok := toBool(strings.TrimSpace(text))
		if !ok {
			return nil, fmt.Errorf("campo %q: valor booleano inválido %q", field.Key, text)
		}
		return b, nil
	case "duration":
		d, ok := toDuration(strings.TrimSpace(text))
		if !ok {
			return nil, fmt.Errorf(
				"campo %q: duração inválida %q (exemplos: 250ms, 5s, 1m30s)",
				field.Key, text)
		}
		return d, nil
	case "list":
		return parseList(text), nil
	default:
		return text, nil
	}
}

// parseList converte uma lista separada por vírgulas em []string.
func parseList(text string) []string {
	if strings.TrimSpace(text) == "" {
		return []string{}
	}
	parts := strings.Split(text, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// editValue devolve o texto inicial da edição inline.
func editValue(field Field, value any, exists bool) string {
	if !exists {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(field.Type)) {
	case "string":
		if s, ok := value.(string); ok {
			return s
		}
	case "int":
		if n, ok := toInt(value); ok {
			return strconv.Itoa(n)
		}
	case "bool":
		if b, ok := toBool(value); ok {
			return strconv.FormatBool(b)
		}
	case "duration":
		if d, ok := toDuration(value); ok {
			return d.String()
		}
	case "list":
		if isList(value) {
			return listText(value)
		}
	}
	return formatValue(value)
}

// displayFieldValue representa o valor de um campo na lista.
func displayFieldValue(field Field, value any, exists bool) string {
	if !exists {
		return "—"
	}
	switch strings.ToLower(strings.TrimSpace(field.Type)) {
	case "string":
		if s, ok := value.(string); ok {
			return s
		}
	case "int":
		if n, ok := toInt(value); ok {
			return strconv.Itoa(n)
		}
	case "bool":
		if b, ok := toBool(value); ok {
			return strconv.FormatBool(b)
		}
	case "duration":
		if d, ok := toDuration(value); ok {
			return d.String()
		}
	case "list":
		if isList(value) {
			return formatList(value)
		}
	}
	return formatValue(value)
}

// listText representa os itens de uma lista sem os colchetes, para edição.
func listText(value any) string {
	if !isList(value) {
		return formatValue(value)
	}
	rv := reflect.ValueOf(value)
	parts := make([]string, 0, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		parts = append(parts, formatListElement(rv.Index(i).Interface()))
	}
	return strings.Join(parts, ", ")
}

// render monta a tela completa.
func (st *editorState) render() string {
	if st.width <= 0 || st.height <= 0 {
		return ""
	}
	lines := make([]string, 0, st.height)
	title := "charm-config  " + st.tabs()
	lines = append(lines, editorTitleStyle.Render(truncateText(title, st.width)))

	listHeight := 0
	if st.height >= 3 {
		listHeight = st.height - 2
	}
	if listHeight > 0 {
		lines = append(lines, st.renderList(listHeight)...)
	}
	if st.height >= 2 {
		style := editorFaintStyle
		if st.confirmQuit || st.statusErr {
			style = editorAlertStyle
		}
		lines = append(lines, style.Render(truncateText(oneLine(st.footer()), st.width)))
	}
	if len(lines) > st.height {
		lines = lines[:st.height]
	}
	return strings.Join(lines, "\n")
}

// tabs devolve o cabeçalho com a aba ativa.
func (st *editorState) tabs() string {
	if st.mode == editorDocs {
		return "valores [docs]"
	}
	return "[valores] docs"
}

// footer devolve a linha de ajuda ou de status.
func (st *editorState) footer() string {
	if st.confirmQuit {
		return "alterações não salvas: q ou y saem, n ou esc cancelam"
	}
	if st.status != "" {
		return st.status
	}
	if st.mode == editorDocs {
		return "↑/↓ navegar · enter editar · tab valores · s salvar · q sair"
	}
	return "↑/↓ navegar · enter editar · tab docs · s salvar · q sair"
}

// renderList renderiza a janela visível da lista de campos.
func (st *editorState) renderList(height int) []string {
	if len(st.schema.Fields) == 0 {
		return []string{truncateText("(nenhum campo)", st.width)}
	}
	total := len(st.schema.Fields)
	start := 0
	if st.index() >= height {
		start = st.index() - height + 1
	}
	if start > total-height {
		start = total - height
	}
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > total {
		end = total
	}
	rows := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		rows = append(rows, st.renderRow(i))
	}
	return rows
}

// renderRow renderiza uma linha da lista.
func (st *editorState) renderRow(index int) string {
	field := st.schema.Fields[index]
	selected := index == st.index()
	marker := "  "
	if selected {
		marker = "> "
	}

	var row string
	switch {
	case st.editing && selected:
		row = marker + field.Key + " = " + st.editDisplay()
	case st.mode == editorDocs:
		doc := oneLine(field.Doc)
		if doc == "" {
			doc = "—"
		}
		row = marker + field.Key + "  " + doc
	default:
		value, exists := lookup(st.values, field.Key)
		row = marker + field.Key + " = " + displayFieldValue(field, value, exists)
		if doc := oneLine(field.Doc); doc != "" {
			used := lipgloss.Width(row)
			if st.width-used >= 14 {
				row += "  " + ansi.Truncate(doc, st.width-used-2, "…")
			}
		}
	}

	row = truncateText(row, st.width)
	if selected {
		row = editorSelectedStyle.Render(row)
	}
	return row
}

// editDisplay mostra o buffer de edição com um cursor visível.
func (st *editorState) editDisplay() string {
	position := st.position
	if position < 0 {
		position = 0
	}
	if position > len(st.buffer) {
		position = len(st.buffer)
	}
	return string(st.buffer[:position]) + "█" + string(st.buffer[position:])
}

// truncateText limita o texto à largura de exibição informada.
func truncateText(text string, width int) string {
	if width <= 0 {
		return ""
	}
	return ansi.Truncate(text, width, "…")
}

// oneLine reduz quebras de linha e espaços repetidos a uma única linha.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
