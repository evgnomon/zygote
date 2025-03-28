ALTER TABLE `{{ .DatabaseName }}`.`{{ .TableName }}`
DROP INDEX `idx_{{ .Name }}`;
