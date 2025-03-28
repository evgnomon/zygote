ALTER TABLE `{{ .DatabaseName }}`.`{{ .TableName }}`
ADD COLUMN `{{ .ColumnName }}` {{ .DataType }} GENERATED ALWAYS AS (data->>'{{ .FieldPath }}'){{ if .Virtual }} VIRTUAL {{ else }} STORED{{ end }};
