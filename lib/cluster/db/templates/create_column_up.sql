{{ if  or (eq .SQLColType "MEDIUMBLOB") (eq .SQLColType "MEDIUMTEXT") (eq .SQLColType "JSON") }}
ALTER TABLE `{{ .DatabaseName }}`.`{{ .TableName }}` ADD COLUMN `{{ .ColumnName }}` {{ .SQLColType }};
{{ else }}
ALTER TABLE `{{ .DatabaseName }}`.`{{ .TableName }}` ADD COLUMN `{{ .ColumnName }}` {{ .SQLColType }} NOT NULL DEFAULT {{ .DefaultValue }};
{{ end }}
