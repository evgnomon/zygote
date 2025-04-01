package container

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"strings"
)

type SQLInitParams struct {
	DBName   string
	Username string
	Password string
}

type RouterConfParams struct {
	Destinations string
}

type InnoDBClusterParams struct {
	ServerID             int
	GroupReplicationPort int
	ServerCount          int
	ServersList          string
	ReportAddress        string
	ReportPort           int
}

//go:embed templates/*
var templates embed.FS

func ApplyTemplate(name string, params any) (string, error) {
	data, err := templates.ReadFile(fmt.Sprintf("templates/%s", name))

	if err != nil {
		return "", err
	}

	tmpl := string(data)

	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return "", err
	}

	var tpl bytes.Buffer
	if err := t.Execute(&tpl, params); err != nil {
		return "", err
	}

	return strings.Trim(tpl.String(), "\n"), nil
}
