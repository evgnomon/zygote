ALTER TABLE `{{ .DatabaseName }}`.`{{ .TableName }}`
ADD {{ if .Unique }}UNIQUE {{ else if .FullText }}FULLTEXT {{end}}INDEX `idx_{{ .Name }}` (
  {{ range $i, $col := .Columns }}{{ if $i }}, {{ end }}`{{ $col }}`{{ end }}
);
