package devnet

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

func ParseKeyAddress(out []byte) (string, error) {
	var payload struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return "", fmt.Errorf("decode key output: %w", err)
	}
	if payload.Address == "" {
		return "", fmt.Errorf("empty address in key output")
	}
	return payload.Address, nil
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination %q: %w", dst, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %q to %q: %w", src, dst, err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close destination %q: %w", dst, err)
	}

	return nil
}
