package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

var customMarker = regexp.MustCompile(`^\$[A-Z][A-Z0-9_]{0,63}$`)
var jsonString = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
var textMarker = regexp.MustCompile(`\$[A-Z][A-Z0-9_]*`)

func SystemVariable(name string) bool {
	switch name {
	case "UUID", "UUID_LIMITED", "USER_UUID", "USER_UUID_LIMITED", "CLIENTS_MAIN", "CLIENTS_LIMITED":
		return true
	}
	return false
}

// Requirements scans markers without parsing a template that may contain raw JSON fragments.
func Requirements(raw string) ([]string, error) {
	if !textMarker.MatchString(raw) {
		if _, err := Decode(raw); err != nil {
			return nil, err
		}
	}
	found := map[string]bool{}
	for _, token := range textMarker.FindAllString(raw, -1) {
		if len(token) > 65 {
			return nil, fmt.Errorf("слишком длинная метка %s", token)
		}
		if !SystemVariable(token[1:]) {
			found[token[1:]] = true
		}
	}
	for _, literal := range jsonString.FindAllString(raw, -1) {
		var value string
		if json.Unmarshal([]byte(literal), &value) == nil && strings.HasPrefix(value, "&") &&
			customMarker.MatchString("$"+value[1:]) && !SystemVariable(value[1:]) {
			found[value[1:]] = true
		}
	}
	result := []string{}
	for name := range found {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

// ValidateTemplate defers JSON validation until all textual variables are substituted.
func ValidateTemplate(raw, kind string) error {
	if kind != "client" && kind != "server" {
		return fmt.Errorf("неизвестный тип шаблона")
	}
	requirements, err := Requirements(raw)
	if err != nil {
		return err
	}
	if textMarker.MatchString(raw) {
		return nil
	}
	dummy := User{UUID: "validation", LimitedUUID: "validation-limited", Enabled: true}
	values := map[string]any{}
	for _, name := range requirements {
		values[name] = "validation"
	}
	_, err = RenderWithVariables(raw, kind, nil, &dummy, values)
	return err
}
func Missing(raw string, values map[string]any) ([]string, error) {
	names, err := Requirements(raw)
	if err != nil {
		return nil, err
	}
	missing := []string{}
	for _, name := range names {
		value, ok := values[name]
		if !ok || value == nil || value == "" {
			missing = append(missing, name)
		}
	}
	return missing, nil
}

type User struct {
	ID                     string `json:"id"`
	Name                   string `json:"name"`
	UUID                   string `json:"uuid"`
	LimitedUUID            string `json:"limited_uuid"`
	SubID                  string `json:"sub_id"`
	ExpiresAt              int64  `json:"expires_at"`
	CreatedAt              int64  `json:"created_at"`
	LimitedBytes           int64  `json:"limited_bytes"`
	Enabled                bool   `json:"enabled"`
	TotalMain              int64  `json:"total_main"`
	TotalLimited           int64  `json:"total_limited"`
	QuotaLimited           int64  `json:"quota_limited"`
	LimitedUsedBytes       *int64 `json:"limited_used_bytes"`
	LastSeenAt             int64  `json:"last_seen_at"`
	SubscriptionLastSeenAt int64  `json:"subscription_last_seen_at"`
	DeviceCount            int    `json:"device_count"`
	UnknownClientCount     int    `json:"unknown_client_count"`
}

func (u User) Active(now time.Time) bool {
	return u.Enabled && (u.ExpiresAt == 0 || now.Unix() < u.ExpiresAt)
}

// Decode preserves numbers and rejects duplicate fields, trailing data and non-object configurations.
func Decode(raw string) (map[string]any, error) {
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	decoded, err := decodeValue(d)
	if err != nil {
		return nil, fmt.Errorf("JSON: %w", err)
	}
	value, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("корень конфига должен быть объектом")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("лишние данные после JSON")
	}
	return value, nil
}

func decodeValue(d *json.Decoder) (any, error) {
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		value := map[string]any{}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("ожидается ключ объекта")
			}
			if _, exists := value[key]; exists {
				return nil, fmt.Errorf("повторяющийся ключ %q: поля объектов автоматически не объединяются", key)
			}
			item, err := decodeValue(d)
			if err != nil {
				return nil, err
			}
			value[key] = item
		}
		_, err := d.Token()
		return value, err
	case json.Delim('['):
		value := []any{}
		for d.More() {
			item, err := decodeValue(d)
			if err != nil {
				return nil, err
			}
			value = append(value, item)
		}
		_, err := d.Token()
		return value, err
	default:
		return token, nil
	}
}

func Render(raw, kind string, users []User, selected *User) ([]byte, error) {
	return RenderWithVariables(raw, kind, users, selected, nil)
}

func RenderWithVariables(raw, kind string, users []User, selected *User, custom map[string]any) ([]byte, error) {
	vars := map[string]any{}
	now := time.Now()
	for name, value := range custom {
		vars["&"+name] = value
		if value != nil && value != "" {
			vars["$"+name] = value
		}
	}
	switch kind {
	case "client":
		if selected == nil || !selected.Active(now) {
			return nil, fmt.Errorf("выберите активного пользователя")
		}
		vars["&UUID"], vars["&UUID_LIMITED"] = selected.UUID, selected.LimitedUUID
		vars["$USER_UUID"], vars["$USER_UUID_LIMITED"] = selected.UUID, selected.LimitedUUID
		vars["$UUID"], vars["$UUID_LIMITED"] = selected.UUID, selected.LimitedUUID
	case "server":
		main, limited := []any{}, []any{}
		for _, u := range users {
			if !u.Active(now) {
				continue
			}
			main = append(main, map[string]any{"id": u.UUID, "email": u.ID + ".main"})
			if u.LimitedBytes == 0 || u.QuotaLimited < u.LimitedBytes {
				limited = append(limited, map[string]any{"id": u.LimitedUUID, "email": u.ID + ".limited"})
			}
		}
		vars["&CLIENTS_MAIN"], vars["&CLIENTS_LIMITED"] = main, limited
		vars["$CLIENTS_MAIN"], vars["$CLIENTS_LIMITED"] = main, limited
	default:
		return nil, fmt.Errorf("неизвестный тип шаблона")
	}
	var substitutionError error
	raw = textMarker.ReplaceAllStringFunc(raw, func(token string) string {
		value, ok := vars[token]
		if !ok || value == nil || value == "" {
			substitutionError = fmt.Errorf("не заполнена переменная %s", token)
			return token
		}
		if text, ok := value.(string); ok {
			return text
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			substitutionError = err
			return token
		}
		return string(encoded)
	})
	if substitutionError != nil {
		return nil, substitutionError
	}
	root, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	value, err := replace(root, vars)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	e := json.NewEncoder(&out)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	err = e.Encode(value)
	return out.Bytes(), err
}

func replace(value any, vars map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		if replacement, ok := vars[v]; ok {
			return replacement, nil
		}
		// Markers occupy a complete JSON string; URLs and arbitrary config strings stay untouched.
		if strings.HasPrefix(v, "&") {
			return nil, fmt.Errorf("неизвестная метка %q: метка должна занимать всё значение строки", v)
		}
	case []any:
		for i, item := range v {
			next, err := replace(item, vars)
			if err != nil {
				return nil, err
			}
			v[i] = next
		}
	case map[string]any:
		for key, item := range v {
			if strings.HasPrefix(key, "&") {
				return nil, fmt.Errorf("метки в ключах JSON не поддерживаются")
			}
			next, err := replace(item, vars)
			if err != nil {
				return nil, err
			}
			v[key] = next
		}
	}
	return value, nil
}
