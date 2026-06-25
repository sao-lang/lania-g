// transaction.go 实现 mongodb 集成的事务封装与会话协调逻辑。
package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"
)

// WithTransaction 在事务中执行给定函数。
func WithTransaction(ctx context.Context, client *mongo.Client, fn func(sessionContext context.Context, session mongo.Session) error) error {
	session, err := client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(sessionContext mongo.SessionContext) (interface{}, error) {
		return nil, fn(sessionContext, session)
	})
	return err
}

// WithSession 在会话中执行给定函数。
func WithSession(ctx context.Context, client *mongo.Client, fn func(sessionContext context.Context, session mongo.Session) error) error {
	session, err := client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	sessionContext := mongo.NewSessionContext(ctx, session)
	return fn(sessionContext, session)
}
