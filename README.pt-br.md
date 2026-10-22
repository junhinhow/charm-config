# charm-config

Configuração em camadas para Go com um editor de terminal Bubble Tea.

`charm-config` combina padrões, arquivos, variáveis de ambiente e flags em uma
única árvore de configuração, expõe getters tipados e um schema para validação.
O editor opcional permite inspecionar e alterar os valores efetivos sem sair do
terminal.

## Instalação

```bash
go get github.com/junhinhow/charm-config
```

## Uso

Três camadas: padrões, um arquivo YAML e variáveis de ambiente.

```go
package main

import (
	"fmt"
	"log"

	tea "charm.land/bubbletea/v2"
	"github.com/junhinhow/charm-config"
)

func main() {
	// config.yaml: server.host=db.internal, server.port=8081
	// APP_SERVER_PORT=9090
	loader := config.New().
		Defaults(map[string]any{
			"server": map[string]any{"host": "localhost", "port": 8080},
		}).
		File("config.yaml").
		Env("APP")

	// config.yaml é opcional neste exemplo; um arquivo ausente ou inválido não
	// vira camada e o erro fica disponível em Err.
	if err := loader.Err(); err != nil {
		log.Printf("config: %v", err)
	}

	fmt.Println(loader.String("server.host", "localhost")) // db.internal
	fmt.Println(loader.Int("server.port", 8080))           // 9090

	schema := config.Schema{Fields: []config.Field{
		{Key: "server.host", Type: "string", Required: true, Doc: "Host do banco"},
		{Key: "server.port", Type: "int", Required: true, Doc: "Porta TCP"},
	}}

	editor := config.NewEditor(loader, schema)
	if _, err := tea.NewProgram(editor).Run(); err != nil {
		log.Fatal(err)
	}
}
```

Exemplo de `config.yaml`:

```yaml
server:
  host: db.internal
  port: 8081
```

A variável `APP_SERVER_PORT=9090` vence o arquivo, e o arquivo vence os padrões.

## Teclas do editor

| Tecla | Ação |
| --- | --- |
| `↑` / `k` | Campo anterior |
| `↓` / `j` | Próximo campo |
| `enter` | Edita o campo selecionado inline |
| `esc` | Cancela a edição atual ou a confirmação de saída |
| `s` | Salva na camada de arquivo de maior prioridade |
| `tab` | Alterna entre valores e documentação |
| `q` | Sai; pede confirmação quando há alterações não salvas |
| `ctrl+c` | Igual a `q` |

A edição inline aceita valores `int`, `bool`, `duration` (`250ms`, `1m30s`) e
`list`. Listas são editadas como itens separados por vírgula.

## API

| Símbolo | Descrição |
| --- | --- |
| `Source{Name, Priority, Data}` | Uma camada de configuração; maior prioridade vence |
| `New()` | Cria um loader vazio |
| `(*Loader).Add(Source)` | Adiciona uma camada personalizada (os dados são copiados) |
| `(*Loader).Defaults(map[string]any)` | Camada com prioridade 0 |
| `(*Loader).File(path)` | Camada JSON/YAML com prioridade 100 |
| `(*Loader).Env(prefix)` | Camada de ambiente com prioridade 200 |
| `(*Loader).Flags(map[string]any)` | Camada de flags com prioridade 300 |
| `(*Loader).Get(key)` | Busca bruta em notação de ponto |
| `(*Loader).String/Int/Bool/Duration` | Busca tipada com valor padrão |
| `(*Loader).All()` | Cópia da configuração com merge profundo |
| `(*Loader).Sources()` | Nomes das camadas na ordem de aplicação |
| `(*Loader).Err()` | Erros de arquivo acumulados |
| `Schema{Fields}` / `(Schema).Validate(all)` | Valida todos os campos e devolve todos os erros |
| `Field{Key, Type, Required, Default, Doc}` | Definição de um campo do schema |
| `NewEditor(loader, schema)` | Cria o modelo Bubble Tea |
| `(*Editor).SetSize(width, height)` | Define a área de desenho |
| `(Editor).Values()` | Valores atuais como mapa aninhado |
| `(Editor).Dirty()` | Indica se há alterações não salvas |
| `(*Editor).Save()` | Grava na camada de arquivo de maior prioridade |
| `ErrNoWritableFile` | Devolvido por `Save` quando não há camada de arquivo |

`Editor` implementa `tea.Model` (`Init`, `Update`, `View`).

## Notas de projeto

- **Merge:** mapas são mesclados recursivamente. Listas e escalares da camada de
  maior prioridade substituem o valor anterior. Prioridades iguais são aplicadas
  na ordem de inclusão, então a última camada adicionada vence.
- **Ambiente:** `APP_DATABASE_HOST` vira `database.host`. Os valores permanecem
  strings na camada e são convertidos pelos getters tipados. Segmentos vazios são
  inválidos; `APP_A__B` é ignorada. Prefixo vazio importa todas as variáveis.
- **Arquivos:** `.json`, `.yaml` e `.yml` são reconhecidos sem diferenciar
  maiúsculas de minúsculas. Um arquivo ausente ou inválido **não** é registrado
  como camada; o erro fica em `Err` e o loader continua utilizável. Várias
  camadas de arquivo são permitidas.
- **Notação de ponto:** `Get("server.port")` percorre mapas aninhados. Chaves
  ausentes devolvem `false`. Listas são valores, não são indexáveis. Uma chave
  literal que contenha ponto não pode ser acessada pela notação de ponto.
- **Validação:** todos os erros são devolvidos na ordem do schema. `int`, `bool`
  e `duration` aceitam strings convertíveis porque camadas de ambiente são
  textuais. `Field.Default` é exibido pelo editor, mas não preenche `All` nem
  satisfaz `Required` na validação.
- **Estado do editor:** o editor captura a configuração mesclada no momento da
  criação. Alterações posteriores no loader exigem um novo editor. `Values`
  devolve uma cópia aninhada.
- **Save:** grava a configuração efetiva na camada de maior prioridade criada por
  `File`, mantendo o formato original (JSON ou YAML) e o modo `0600`. Os valores
  mesclados de todas as camadas são materializados nesse arquivo, portanto o save
  não é um diff mínimo. Durações são gravadas como strings, por exemplo `"5s"`.
  As chaves são ordenadas pelos codificadores JSON/YAML. Comentários e a
  formatação original não são preservados.
- **Listas no editor:** separadas por vírgula. Edições inline gravam os itens
  como strings. Um item que contenha vírgula não pode ser representado pelo
  editor inline.
- **Renderização:** a ordem do schema é preservada, valores de mapa exibidos no
  editor usam a ordenação JSON e nenhum mapa é iterado para montar uma linha.
  `View` nunca ultrapassa a largura/altura configurada; dimensões zero ou
  negativas produzem uma view vazia.

## Licença

MIT
