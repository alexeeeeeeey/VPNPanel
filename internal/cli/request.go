package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
)

func request(args []string) (method, path string, payload []byte, err error) {
	var data map[string]any
	if len(args) == 0 {
		return "", "", nil, errors.New(Usage)
	}
	switch args[0] {
	case "state":
		if len(args) == 1 {
			return "GET", "state", nil, nil
		}
	case "user-add":
		if len(args) == 2 {
			method, path = "POST", "users"
			data = map[string]any{"name": args[1]}
		}
	case "user-enable", "user-disable":
		if len(args) == 2 && userID.MatchString(args[1]) {
			method, path = "PATCH", "users/"+args[1]
			data = map[string]any{"enabled": args[0] == "user-enable"}
		}
	case "template-save", "render":
		saving := args[0] == "template-save"
		minArgs := 3
		if saving {
			minArgs = 4
		}
		if len(args) < minArgs || len(args) > minArgs+1 || (args[1] != "client" && args[1] != "server") {
			break
		}
		id := ""
		if len(args) > minArgs {
			id = args[minArgs]
		}
		if (id != "" && !userID.MatchString(id)) || (!saving && ((args[1] == "client") != (id != ""))) {
			break
		}
		body, readErr := readTemplate(args[minArgs-1])
		if readErr != nil {
			return "", "", nil, readErr
		}
		data = map[string]any{"kind": args[1], "body": body}
		method, path = "POST", "render"
		if saving {
			path = "templates"
			data["name"], data["id"] = args[2], id
		} else {
			data["user_id"] = id
		}
	}
	if data == nil {
		return "", "", nil, errors.New(Usage)
	}
	payload, err = json.Marshal(data)
	return
}

func readTemplate(path string) (string, error) {
	input := os.Stdin
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()
		input = file
	}
	const limit = 2 << 20
	body, err := io.ReadAll(io.LimitReader(input, limit+1))
	if err == nil && len(body) > limit {
		err = errors.New("template exceeds 2 MiB")
	}
	return string(body), err
}
