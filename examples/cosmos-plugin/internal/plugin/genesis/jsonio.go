package genesis

import (
	"encoding/json"
	"fmt"
	"io"
)

func WriteObjectKey(w io.Writer, first *bool, key string) error {
	if !*first {
		if _, err := io.WriteString(w, ","); err != nil {
			return err
		}
	}
	*first = false

	b, err := json.Marshal(key)
	if err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err = io.WriteString(w, ":")
	return err
}

func WriteRawJSONValue(dec *json.Decoder, w io.Writer) error {
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	_, err := w.Write(raw)
	return err
}

func DiscardJSONValue(dec *json.Decoder) error {
	var raw json.RawMessage
	return dec.Decode(&raw)
}

func WriteJSONValue(w io.Writer, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func ReadObjectValue(dec *json.Decoder) (map[string]any, error) {
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func EnsureMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

// CopyJSONValue streams one JSON value from decoder to writer without buffering
// the full value into a single RawMessage.
func CopyJSONValue(dec *json.Decoder, w io.Writer) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	return writeTokenValue(dec, w, tok)
}

// SkipJSONValue discards one JSON value from decoder without buffering the full
// value into a single RawMessage.
func SkipJSONValue(dec *json.Decoder) error {
	return CopyJSONValue(dec, io.Discard)
}

func writeTokenValue(dec *json.Decoder, w io.Writer, tok json.Token) error {
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			if _, err := io.WriteString(w, "{"); err != nil {
				return err
			}

			first := true
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyTok.(string)
				if !ok {
					return fmt.Errorf("object key token must be string, got %T", keyTok)
				}
				if err := WriteObjectKey(w, &first, key); err != nil {
					return err
				}
				if err := CopyJSONValue(dec, w); err != nil {
					return err
				}
			}

			endTok, err := dec.Token()
			if err != nil {
				return err
			}
			endDelim, ok := endTok.(json.Delim)
			if !ok || endDelim != '}' {
				return fmt.Errorf("expected object terminator, got %T %v", endTok, endTok)
			}

			_, err = io.WriteString(w, "}")
			return err
		case '[':
			if _, err := io.WriteString(w, "["); err != nil {
				return err
			}

			first := true
			for dec.More() {
				if !first {
					if _, err := io.WriteString(w, ","); err != nil {
						return err
					}
				}
				first = false

				if err := CopyJSONValue(dec, w); err != nil {
					return err
				}
			}

			endTok, err := dec.Token()
			if err != nil {
				return err
			}
			endDelim, ok := endTok.(json.Delim)
			if !ok || endDelim != ']' {
				return fmt.Errorf("expected array terminator, got %T %v", endTok, endTok)
			}

			_, err = io.WriteString(w, "]")
			return err
		default:
			return fmt.Errorf("unsupported delimiter %q", t)
		}
	default:
		b, err := json.Marshal(tok)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
}
