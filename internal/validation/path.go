package validation

import (
	"regexp"
)

var Path = regexp.MustCompile(`^(/[a-zA-Z0-9_-]+)+$`)
var SubID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
