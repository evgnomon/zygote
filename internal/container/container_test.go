package container

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyTemplate(t *testing.T) {
	tests := []struct {
		templateName string
		params       any
		want         []string
	}{
		{
			templateName: "sql_init_template.sql",
			params:       SQLInitParams{DBName: "myproject_1", Username: "test_1", Password: "password"},
			want: []string{
				"CREATE DATABASE IF NOT EXISTS myproject_1;",
				"CREATE USER IF NOT EXISTS 'test_1'@'localhost' IDENTIFIED WITH mysql_native_password BY 'password';",
				"CREATE USER IF NOT EXISTS 'test_1'@'%' IDENTIFIED WITH mysql_native_password BY 'password';",
				"FLUSH PRIVILEGES;",
				"GRANT ALL PRIVILEGES ON *.* TO 'test_1'@'localhost' WITH GRANT OPTION;",
				"GRANT ALL PRIVILEGES ON *.* TO 'test_1'@'%' WITH GRANT OPTION;",
				"FLUSH PRIVILEGES;",
			},
		},
		{
			templateName: "sql_init_template.sql",
			params:       SQLInitParams{DBName: "myproject_2", Username: "test_2", Password: "password"},
			want: []string{
				"CREATE DATABASE IF NOT EXISTS myproject_2;",
				"CREATE USER IF NOT EXISTS 'test_2'@'localhost' IDENTIFIED WITH mysql_native_password BY 'password';",
				"CREATE USER IF NOT EXISTS 'test_2'@'%' IDENTIFIED WITH mysql_native_password BY 'password';",
				"FLUSH PRIVILEGES;",
				"GRANT ALL PRIVILEGES ON *.* TO 'test_2'@'localhost' WITH GRANT OPTION;",
				"GRANT ALL PRIVILEGES ON *.* TO 'test_2'@'%' WITH GRANT OPTION;",
				"FLUSH PRIVILEGES;",
			},
		},
		{
			templateName: "innodb_cluster_template.cnf",
			params: InnoDBClusterParams{
				ServerID:             1,
				GroupReplicationPort: 33061,
				ServerCount:          3,
				ServersList:          "zygote-db-rep-1:33061,zygote-db-rep-2:33061,zygote-db-rep-3:33061",
				ReportAddress:        "zygote-db-rep-1",
				ReportPort:           1234,
			},
			want: []string{
				"[mysqld]",
				"server_id=1",
				"bind-address = 0.0.0.0",
				`report_host = "zygote-db-rep-1"`,
				"report_port = 1234",
				"gtid_mode=ON",
				"enforce_gtid_consistency=ON",
				"log_replica_updates=ON",
				"log-bin=mysql-bin",
				"binlog_format=ROW",
				"xa_detach_on_prepare=ON",
				"binlog_checksum=NONE",
				"relay-log = mysqld-relay-bin",
				"relay-log-index = mysqld-relay-bin.index",
				"mysql_native_password=ON",
				"sql_require_primary_key=ON",
				`disabled_storage_engines="MyISAM,BLACKHOLE,FEDERATED,ARCHIVE,MEMORY"`,
				"max_connections = 300",
				"mysqlx_max_connections = 200",
			},
		},
		{
			templateName: "innodb_cluster_template.cnf",
			params: InnoDBClusterParams{
				ServerID:             2,
				GroupReplicationPort: 33061,
				ServerCount:          3,
				ServersList:          "shard-a.zygote.com:33061,shard-a.zygote.com:33061,shard-c.zygote.com:33061",
				ReportAddress:        "shard-b.zygote.com",
				ReportPort:           3306,
			},
			want: []string{
				"[mysqld]",
				"server_id=2",
				"bind-address = 0.0.0.0",
				`report_host = "shard-b.zygote.com"`,
				`report_port = 3306`,
				"gtid_mode=ON",
				"enforce_gtid_consistency=ON",
				"log_replica_updates=ON",
				"log-bin=mysql-bin",
				"binlog_format=ROW",
				"xa_detach_on_prepare=ON",
				"binlog_checksum=NONE",
				"relay-log = mysqld-relay-bin",
				"relay-log-index = mysqld-relay-bin.index",
				"mysql_native_password=ON",
				"sql_require_primary_key=ON",
				`disabled_storage_engines="MyISAM,BLACKHOLE,FEDERATED,ARCHIVE,MEMORY"`,
				"max_connections = 300",
				"mysqlx_max_connections = 200",
			},
		},
	}

	for _, tt := range tests {
		result, err := ApplyTemplate(tt.templateName, tt.params)
		assert.Nil(t, err, "Error should be nil")
		assert.Equal(t, result, strings.Join(tt.want, "\n"))
	}
}
