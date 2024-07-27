package container

import (
	"bytes"
	"embed"
	"html/template"
	"strings"
)

type SQLInitParams struct {
	DBName   string
	Username string
	Password string
}

//go:embed templates/sql_init_template.sql
var content embed.FS

func SQLInit(params SQLInitParams) (string, error) {
	data, err := content.ReadFile("templates/sql_init_template.sql")

	if err != nil {
		return "", err
	}

	tmpl := string(data)

	t, err := template.New("sqlInit").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var tpl bytes.Buffer
	if err := t.Execute(&tpl, params); err != nil {
		return "", err
	}

	return strings.Trim(tpl.String(), "\n"), nil
}
