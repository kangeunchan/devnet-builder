package cosmos

import (
	"encoding/json"
	"io"
)

func writeObjectKey(w io.Writer, first *bool, key string) error {
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

func writeRawJSONValue(dec *json.Decoder, w io.Writer) error {
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	_, err := w.Write(raw)
	return err
}

func discardJSONValue(dec *json.Decoder) error {
	var raw json.RawMessage
	return dec.Decode(&raw)
}

func writeJSONValue(w io.Writer, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func readObjectValue(dec *json.Decoder) (map[string]any, error) {
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func ensureMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}
