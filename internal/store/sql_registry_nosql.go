//go:build !db_sql

package store

import "fmt"

func newSQLStorage(driver, dsn string) (Storage, error) {
	return nil, fmt.Errorf("драйвер %q требует сборки с тегом db_sql: go build -tags db_sql . (сейчас доступен драйвер json)", driver)
}