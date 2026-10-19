package mongodb

import (


)

type analyticsDB struct{}

func (analyticsDB) MongoDatabaseName() string { return "analytics" }

