CREATE DATABASE IF NOT EXISTS {{.DBName}};
CREATE USER IF NOT EXISTS '{{.Username}}'@'localhost' IDENTIFIED WITH mysql_native_password BY '{{.Password}}';
CREATE USER IF NOT EXISTS '{{.Username}}'@'%' IDENTIFIED WITH mysql_native_password BY '{{.Password}}';
FLUSH PRIVILEGES;
GRANT ALL PRIVILEGES ON {{.DBName}}.* TO '{{.Username}}'@'localhost' WITH GRANT OPTION;
GRANT ALL PRIVILEGES ON {{.DBName}}.* TO '{{.Username}}'@'%' WITH GRANT OPTION;
FLUSH PRIVILEGES;
