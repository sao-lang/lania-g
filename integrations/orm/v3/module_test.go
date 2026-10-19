package orm

import (


	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/migrator"
	"gorm.io/gorm/schema"
)

type testUser struct {
	ID   uint
	Name string
}

type analyticsDB struct{}

func (analyticsDB) ORMDataSourceName() string { return "analytics" }

type fakeDialector struct{}

func (fakeDialector) Name() string { return "fake" }

func (fakeDialector) Initialize(db *gorm.DB) error { return nil }

func (fakeDialector) Migrator(db *gorm.DB) gorm.Migrator { return migrator.Migrator{} }

func (fakeDialector) DataTypeOf(*schema.Field) string { return "" }

func (fakeDialector) DefaultValueOf(*schema.Field) clause.Expression { return clause.Expr{} }

func (fakeDialector) BindVarTo(writer clause.Writer, stmt *gorm.Statement, v interface{}) {
	writer.WriteByte('?')
}

func (fakeDialector) QuoteTo(writer clause.Writer, s string) { writer.WriteString(s) }

func (fakeDialector) Explain(sql string, vars ...interface{}) string { return sql }

