//go:build db_sql

package store

func newSQLStorage(driver, dsn string) (Storage, error) {
	return NewSQLStorage(driver, dsn)
}