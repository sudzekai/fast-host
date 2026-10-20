package store

import "fmt"

func NewStorage(driver, dsn string) (Storage, error) {
	switch driver {
	case "json":
		return NewJSONStorage(dsn)
	case "sqlite", "mysql":
		return newSQLStorage(driver, dsn)
	default:
		return nil, fmt.Errorf("unknown db driver %q (use json | sqlite | mysql)", driver)
	}
}