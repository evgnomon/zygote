package container

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSqlInit(t *testing.T) {
	tests := []struct {
		params SQLInitParams
		want   []string
	}{
		{
			params: SQLInitParams{DBName: "myproject_1", Username: "test_1", Password: "password"},
			want: []string{
				"CREATE DATABASE IF NOT EXISTS myproject_1;",
				"CREATE USER IF NOT EXISTS 'test_1'@'localhost' IDENTIFIED WITH mysql_native_password BY 'password';",
				"CREATE USER IF NOT EXISTS 'test_1'@'%' IDENTIFIED WITH mysql_native_password BY 'password';",
				"FLUSH PRIVILEGES;",
				"GRANT ALL PRIVILEGES ON myproject_1.* TO 'test_1'@'localhost' WITH GRANT OPTION;",
				"GRANT ALL PRIVILEGES ON myproject_1.* TO 'test_1'@'%' WITH GRANT OPTION;",
				"FLUSH PRIVILEGES;",
			},
		},
		{
			params: SQLInitParams{DBName: "myproject_2", Username: "test_2", Password: "password"},
			want: []string{
				"CREATE DATABASE IF NOT EXISTS myproject_2;",
				"CREATE USER IF NOT EXISTS 'test_2'@'localhost' IDENTIFIED WITH mysql_native_password BY 'password';",
				"CREATE USER IF NOT EXISTS 'test_2'@'%' IDENTIFIED WITH mysql_native_password BY 'password';",
				"FLUSH PRIVILEGES;",
				"GRANT ALL PRIVILEGES ON myproject_2.* TO 'test_2'@'localhost' WITH GRANT OPTION;",
				"GRANT ALL PRIVILEGES ON myproject_2.* TO 'test_2'@'%' WITH GRANT OPTION;",
				"FLUSH PRIVILEGES;",
			},
		},
	}

	for _, tt := range tests {
		result, err := SQLInit(tt.params)
		assert.Nil(t, err, "Error should be nil")
		assert.Equal(t, strings.Join(tt.want, "\n"), result)
	}
}
