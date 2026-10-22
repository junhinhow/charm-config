# charm-config

Layered configuration for Go with a Bubble Tea terminal editor.

`charm-config` merges defaults, files, environment variables and flags into one
configuration tree, then exposes typed getters and a schema for validation. The
optional editor lets you inspect and change the effective values without
leaving the terminal.

## Install

```bash
go get github.com/junhinhow/charm-config
```

## Usage

Three layers: defaults, a YAML file and environment variables.

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

	// config.yaml is optional in this example; a missing or invalid file is
	// not registered as a layer and is reported by Err.
	if err := loader.Err(); err != nil {
		log.Printf("config: %v", err)
	}

	fmt.Println(loader.String("server.host", "localhost")) // db.internal
	fmt.Println(loader.Int("server.port", 8080))           // 9090

	schema := config.Schema{Fields: []config.Field{
		{Key: "server.host", Type: "string", Required: true, Doc: "Database host"},
		{Key: "server.port", Type: "int", Required: true, Doc: "TCP port"},
	}}

	editor := config.NewEditor(loader, schema)
	if _, err := tea.NewProgram(editor).Run(); err != nil {
		log.Fatal(err)
	}
}
```

Example `config.yaml`:

```yaml
server:
  host: db.internal
  port: 8081
```

The environment variable `APP_SERVER_PORT=9090` wins over the file, and the
file wins over defaults.

## Editor keys

| Key | Action |
| --- | --- |
| `↑` / `k` | Previous field |
| `↓` / `j` | Next field |
| `enter` | Edit the selected field inline |
| `esc` | Cancel the current edit or quit confirmation |
| `s` | Save to the highest-priority file layer |
| `tab` | Toggle between values and docs |
| `q` | Quit; asks for confirmation when there are unsaved changes |
| `ctrl+c` | Same as `q` |

Inline editing accepts `int`, `bool`, `duration` (`250ms`, `1m30s`) and `list`
values. Lists are edited as comma-separated items.

## API

| Symbol | Description |
| --- | --- |
| `Source{Name, Priority, Data}` | One configuration layer; higher priority wins |
| `New()` | Create an empty loader |
| `(*Loader).Add(Source)` | Add a custom layer (data is copied) |
| `(*Loader).Defaults(map[string]any)` | Layer at priority 0 |
| `(*Loader).File(path)` | JSON/YAML layer at priority 100 |
| `(*Loader).Env(prefix)` | Environment layer at priority 200 |
| `(*Loader).Flags(map[string]any)` | Flags layer at priority 300 |
| `(*Loader).Get(key)` | Raw lookup with dot notation |
| `(*Loader).String/Int/Bool/Duration` | Typed lookup with a default |
| `(*Loader).All()` | Deep-merged configuration copy |
| `(*Loader).Sources()` | Layer names in application order |
| `(*Loader).Err()` | Accumulated file errors |
| `Schema{Fields}` / `(Schema).Validate(all)` | Validate all fields and return every error |
| `Field{Key, Type, Required, Default, Doc}` | Schema field definition |
| `NewEditor(loader, schema)` | Create the Bubble Tea model |
| `(*Editor).SetSize(width, height)` | Set the drawing area |
| `(Editor).Values()` | Current values as a nested map |
| `(Editor).Dirty()` | Whether there are unsaved changes |
| `(*Editor).Save()` | Write to the highest-priority file layer |
| `ErrNoWritableFile` | Returned by `Save` when there is no file layer |

`Editor` implements `tea.Model` (`Init`, `Update`, `View`).

## Design notes

- **Merge:** maps are merged recursively. Lists and scalars from a higher
  priority layer replace the previous value. Equal priorities are applied in
  insertion order, so the last added layer wins.
- **Environment:** `APP_DATABASE_HOST` becomes `database.host`. Values stay as
  strings in the layer and are converted by the typed getters. Segments must be
  non-empty; `APP_A__B` is ignored. An empty prefix imports all variables.
- **Files:** `.json`, `.yaml` and `.yml` are recognized case-insensitively. A
  missing or invalid file is **not** registered as a layer; the error is kept in
  `Err` and the loader remains usable. Multiple file layers are allowed.
- **Dot notation:** `Get("server.port")` walks nested maps. Missing keys return
  `false`. Lists are values, not addressable by index. A literal map key that
  contains a dot cannot be reached with dot notation.
- **Validation:** all errors are returned in schema order. `int`, `bool` and
  `duration` accept convertible strings because environment layers are textual.
  `Field.Default` is shown by the editor but does not fill `All` and does not
  satisfy `Required` during validation.
- **Editor state:** the editor takes a snapshot of the merged configuration when
  it is created. Later loader changes require a new editor. `Values` returns a
  nested copy.
- **Save:** writes the effective configuration to the highest-priority layer
  created by `File`, using the original format (JSON or YAML) and mode `0600`.
  The merged values of all layers are materialized in that file, so save is not
  a minimal diff. Durations are written as strings such as `"5s"`. Keys are
  sorted by the JSON/YAML encoders. Comments and the original formatting are not
  preserved.
- **Lists in the editor:** comma-separated. Inline edits store the items as
  strings. An item that itself contains a comma cannot be represented by the
  inline editor.
- **Rendering:** schema order is preserved, map values rendered in the editor use
  JSON ordering, and no map is iterated to build a line. `View` never exceeds the
  configured width/height; zero or negative dimensions produce an empty view.

## License

MIT
