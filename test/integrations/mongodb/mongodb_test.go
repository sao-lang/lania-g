package mongodb_test

import ()

type analyticsDB struct{}

func (analyticsDB) MongoDatabaseName() string { return "analytics" }
