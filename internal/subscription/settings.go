package subscription

import (
	"errors"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"

	"vpnpanel/internal/validation"
)

type Settings struct {
	Enabled     bool   `json:"enabled"`
	IP          string `json:"ip"`
	Port        int    `json:"port"`
	Path        string `json:"path"`
	PublicURL   string `json:"public_url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Support     string `json:"support"`
	Website     string `json:"website"`
	Interval    int    `json:"interval"`
}

func (settings Settings) validate() error {
	if net.ParseIP(settings.IP) == nil || settings.Port < 1 || settings.Port > 65535 || !validation.Path.MatchString(settings.Path) {
		return errors.New("Укажите IP, порт и путь, например /sub")
	}
	if utf8.RuneCountInString(settings.Title) > 25 || utf8.RuneCountInString(settings.Description) > 200 || settings.Interval < 0 || settings.Interval > 8760 {
		return errors.New("Название: до 25 символов, описание: до 200, интервал: от 0 до 8760 часов")
	}
	for _, value := range []string{settings.PublicURL, settings.Support, settings.Website} {
		if value == "" {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || strings.ContainsAny(value, "\r\n") {
			return errors.New("Ожидается ссылка HTTP или HTTPS")
		}
		if value == settings.PublicURL && (u.RawQuery != "" || u.ForceQuery || u.Fragment != "") {
			return errors.New("Публичный URL должен быть без query и фрагмента")
		}
	}
	return nil
}
