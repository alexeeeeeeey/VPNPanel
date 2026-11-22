package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func Write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func Fail(w http.ResponseWriter, status int, message string) {
	Write(w, status, map[string]string{"error": message})
}
func Read(w http.ResponseWriter, r *http.Request, value any) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		Fail(w, 400, "Некорректный запрос: "+err.Error())
		return false
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		Fail(w, 400, "Лишние данные в запросе")
		return false
	}
	return true
}
