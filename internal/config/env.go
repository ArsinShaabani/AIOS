package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Load reads YAML (when path is non-empty), applies AIOS_* environment
// overrides and validates the result. Empty path => defaults + env only.
func Load(path string) (Config, error) {
	cfg := Default()
	if path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config %q: %w", path, err)
		}
		raw = []byte(expandPlaceholders(string(raw)))
		dec := newStrictDecoder(raw)
		if err := dec.Decode(&cfg); err != nil {
			return cfg, fmt.Errorf("parse config %q: %w", path, err)
		}
	}
	applyEnv(reflect.ValueOf(&cfg).Elem(), "AIOS")
	if err := cfg.Normalize(); err != nil {
		return cfg, err
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// applyEnv walks the config struct and maps every leaf to
// AIOS_<PATH> (e.g. server.addr -> AIOS_SERVER_ADDR). Slices of structs and maps
// are intentionally skipped; those are YAML/file concerns.
func applyEnv(v reflect.Value, prefix string) {
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		structField := t.Field(i)
		if !structField.IsExported() {
			continue
		}
		name := strings.Split(structField.Tag.Get("yaml"), ",")[0]
		if name == "" || name == "-" {
			name = structField.Name
		}
		envName := prefix + "_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))

		// Duration is an int64 underneath, so it must be handled before the
		// numeric cases (otherwise "90s" would be parsed as an integer).
		if field.Type() == reflect.TypeOf(Duration(0)) {
			if raw, ok := os.LookupEnv(envName); ok {
				if err := setDuration(field, raw); err != nil {
					fmt.Fprintf(os.Stderr, "config: invalid %s=%q: %v\n", envName, raw, err)
				}
			}
			continue
		}

		switch field.Kind() {
		case reflect.Struct:
			applyEnv(field, envName)
		case reflect.Pointer:
			if field.Type().Elem().Kind() == reflect.Bool {
				if raw, ok := os.LookupEnv(envName); ok {
					b, err := strconv.ParseBool(raw)
					if err != nil {
						fmt.Fprintf(os.Stderr, "config: invalid %s=%q: %v\n", envName, raw, err)
						continue
					}
					p := reflect.New(field.Type().Elem())
					p.Elem().SetBool(b)
					field.Set(p)
				}
			}
		case reflect.Slice:
			if field.Type().Elem().Kind() != reflect.String {
				continue
			}
			if raw, ok := os.LookupEnv(envName); ok {
				parts := splitList(raw)
				slice := reflect.MakeSlice(field.Type(), len(parts), len(parts))
				for j, p := range parts {
					slice.Index(j).SetString(p)
				}
				field.Set(slice)
			}
		case reflect.String:
			if raw, ok := os.LookupEnv(envName); ok {
				field.SetString(raw)
			}
		case reflect.Bool:
			if raw, ok := os.LookupEnv(envName); ok {
				if b, err := strconv.ParseBool(raw); err == nil {
					field.SetBool(b)
				} else {
					fmt.Fprintf(os.Stderr, "config: invalid %s=%q: %v\n", envName, raw, err)
				}
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if raw, ok := os.LookupEnv(envName); ok {
				if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
					field.SetInt(n)
				} else {
					fmt.Fprintf(os.Stderr, "config: invalid %s=%q: %v\n", envName, raw, err)
				}
			}
		case reflect.Uint8:
			if raw, ok := os.LookupEnv(envName); ok {
				if n, err := strconv.ParseUint(raw, 10, 8); err == nil {
					field.SetUint(n)
				} else {
					fmt.Fprintf(os.Stderr, "config: invalid %s=%q: %v\n", envName, raw, err)
				}
			}
		case reflect.Float64, reflect.Float32:
			if raw, ok := os.LookupEnv(envName); ok {
				if f, err := strconv.ParseFloat(raw, 64); err == nil {
					field.SetFloat(f)
				} else {
					fmt.Fprintf(os.Stderr, "config: invalid %s=%q: %v\n", envName, raw, err)
				}
			}
		}
	}
}

func setDuration(v reflect.Value, raw string) error {
	var d Duration
	if err := d.UnmarshalText([]byte(raw)); err != nil {
		return err
	}
	v.Set(reflect.ValueOf(d))
	return nil
}

func splitList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
