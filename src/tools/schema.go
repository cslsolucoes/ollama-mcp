package tools

// Schema constructors (TOOL-004): every inputSchema is built here, never as a
// hand-written literal per tool, so type, properties, required and
// additionalProperties:false are uniform.

type schemaMap = map[string]any

func objectSchema(required []string, props schemaMap) schemaMap {
	s := schemaMap{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func stringProp(desc string) schemaMap {
	return schemaMap{"type": "string", "description": desc}
}

func nonEmptyStringProp(desc string) schemaMap {
	return schemaMap{"type": "string", "minLength": 1, "description": desc}
}

func numberProp(desc string, min, max float64) schemaMap {
	return schemaMap{"type": "number", "minimum": min, "maximum": max, "description": desc}
}

func intProp(desc string, min, max int) schemaMap {
	return schemaMap{"type": "integer", "minimum": min, "maximum": max, "description": desc}
}

func boolProp(desc string) schemaMap {
	return schemaMap{"type": "boolean", "description": desc}
}

// thinkProp is the optional per-call reasoning switch (plan 8.8b).
func thinkProp() schemaMap {
	return boolProp("Reason first: slower; for hard analysis. Default false.")
}

func arrayProp(desc string, items schemaMap) schemaMap {
	return schemaMap{"type": "array", "items": items, "description": desc}
}
