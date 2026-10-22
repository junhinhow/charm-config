package config

import (
	"fmt"
	"strings"
)

// Field descreve um campo esperado na configuração.
type Field struct {
	// Key é a chave em notação de ponto, como "server.port".
	Key string
	// Type é o tipo esperado: "string", "int", "bool", "duration" ou "list".
	Type string
	// Required exige que a chave exista em [Schema.Validate].
	Required bool
	// Default é o valor inicial exibido pelo editor quando a chave não existe.
	// A validação não usa Default para preencher valores ausentes.
	Default any
	// Doc descreve o campo no editor.
	Doc string
}

// Schema descreve e valida um conjunto de campos.
//
// A ordem de [Schema.Fields] é preservada pelo editor e pela validação.
type Schema struct {
	// Fields são os campos do schema, na ordem de exibição.
	Fields []Field
}

// Validate verifica presença, tipo e obrigatoriedade de todos os campos.
//
// A validação devolve todos os erros encontrados, na ordem dos campos, e não
// apenas o primeiro. Valores textuais convertíveis são aceitos para int, bool
// e duration, porque camadas de ambiente mantêm os valores como string.
// Default não preenche valores ausentes: um campo Required sem valor gera
// erro mesmo que tenha Default.
func (s Schema) Validate(all map[string]any) []error {
	var errs []error
	for _, field := range s.Fields {
		typ := strings.ToLower(strings.TrimSpace(field.Type))
		if !knownFieldType(typ) {
			errs = append(errs, fmt.Errorf("campo %q: tipo desconhecido %q", field.Key, field.Type))
			continue
		}

		value, ok := lookup(all, field.Key)
		if !ok {
			if field.Required {
				errs = append(errs, fmt.Errorf(
					"campo %q é obrigatório (esperado %s)", field.Key, typ))
			}
			continue
		}
		if !validFieldType(typ, value) {
			errs = append(errs, fmt.Errorf(
				"campo %q: esperado %s, obtido %s", field.Key, typ, describeValue(value)))
		}
	}
	return errs
}

// knownFieldType informa se o tipo é suportado pelo schema.
func knownFieldType(typ string) bool {
	switch typ {
	case "string", "int", "bool", "duration", "list":
		return true
	default:
		return false
	}
}

// validFieldType verifica se o valor atende ao tipo informado.
func validFieldType(typ string, value any) bool {
	switch typ {
	case "string":
		_, ok := value.(string)
		return ok
	case "int":
		_, ok := toInt(value)
		return ok
	case "bool":
		_, ok := toBool(value)
		return ok
	case "duration":
		_, ok := toDuration(value)
		return ok
	case "list":
		return isList(value)
	default:
		return false
	}
}

// describeValue descreve o valor recebido sem imprimir mapas ou listas
// inteiras, o que poderia variar entre execuções.
func describeValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "nil"
	case string:
		return fmt.Sprintf("string %q", v)
	case bool:
		return fmt.Sprintf("bool (%t)", v)
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprintf("%T (%v)", value, value)
	default:
		return fmt.Sprintf("%T", value)
	}
}
