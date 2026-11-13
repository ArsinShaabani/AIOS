package config

import (
	"bytes"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

func newStrictDecoder(raw []byte) *yaml.Decoder {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true) // reject typos instead of silently ignoring them
	return dec
}

var placeholderRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandPlaceholders substitutes ${ENV_VAR} in the YAML document, which keeps
// secrets out of committed config files.
func expandPlaceholders(s string) string {
	return placeholderRe.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-1]
		return os.Getenv(name)
	})
}

// String renders the effective configuration as YAML (used by `aios config`).
func (c Config) String() string {
	out, err := yaml.Marshal(c)
	if err != nil {
		return ""
	}
	return string(out)
}
