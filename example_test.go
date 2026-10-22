package config_test

import (
	"fmt"

	config "github.com/junhinhow/charm-config"
)

// ExampleLoader mostra três camadas: padrões, arquivo e flags.
func ExampleLoader() {
	loader := config.New().
		Defaults(map[string]any{
			"server": map[string]any{"host": "localhost", "port": 8080},
		}).
		Add(config.Source{
			Name:     "arquivo",
			Priority: 100,
			Data: map[string]any{
				"server": map[string]any{"host": "db.interno"},
			},
		}).
		Flags(map[string]any{
			"server": map[string]any{"port": 9090},
		})

	fmt.Printf("%s:%d\n", loader.String("server.host", ""), loader.Int("server.port", 0))
	// Output: db.interno:9090
}

// ExampleSchema_validate mostra a mensagem de um tipo inválido.
func ExampleSchema_validate() {
	schema := config.Schema{Fields: []config.Field{
		{Key: "server.port", Type: "int", Required: true, Doc: "Porta TCP"},
	}}
	errs := schema.Validate(map[string]any{
		"server": map[string]any{"port": "abc"},
	})
	fmt.Println(len(errs), errs[0])
	// Output: 1 campo "server.port": esperado int, obtido string "abc"
}

// ExampleEditor mostra a criação do modelo e a leitura dos valores.
func ExampleEditor() {
	loader := config.New().Defaults(map[string]any{
		"server": map[string]any{"port": 8080},
	})
	schema := config.Schema{Fields: []config.Field{
		{Key: "server.port", Type: "int", Doc: "Porta TCP"},
	}}
	editor := config.NewEditor(loader, schema)
	editor.SetSize(40, 8)

	server := editor.Values()["server"].(map[string]any)
	fmt.Println(server["port"], editor.Dirty())
	// Output: 8080 false
}
