CREATE TABLE IF NOT EXISTS `{{ .DatabaseName }}`.`{{ .TableName }}` (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    data JSON NOT NULL,
    PRIMARY KEY (id)
);
