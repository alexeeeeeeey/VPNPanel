package cli

import "regexp"

const Usage = `Usage: vpnpanelctl -host SSH_ALIAS [-port 9080] [-base-path /panel] [-auth-file /etc/vpnpanel/cli.headers] COMMAND

  state
  user-add NAME
  user-enable ID
  user-disable ID
  template-save client|server NAME FILE [ID]
  render client|server FILE [USER_ID]

FILE can be - to read a template from stdin. Responses are JSON.`

var userID = regexp.MustCompile(`^[0-9A-Fa-f]{8}(-[0-9A-Fa-f]{4}){3}-[0-9A-Fa-f]{12}$`)
var basePath = regexp.MustCompile(`^(/[a-zA-Z0-9_-]+)*$`)
var authPath = regexp.MustCompile(`^/[A-Za-z0-9_./-]+$`)
