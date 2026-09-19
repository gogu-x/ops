package mongorpc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogu-x/tree"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const DefaultActorName = "ops-mongo"

type InsertOne struct {
	Collection string
	Doc        interface{}
}
type FindOne struct {
	Collection string
	Filter     interface{}
	Result     interface{}
}
type FindMany struct {
	Collection string
	Filter     interface{}
	Results    interface{}
}
type Count struct {
	Collection string
	Filter     interface{}
}
type UpdateOne struct {
	Collection string
	Filter     interface{}
	Update     interface{}
	Upsert     bool
}
type ReplaceOne struct {
	Collection  string
	Filter      interface{}
	Replacement interface{}
}
type DeleteOne struct {
	Collection string
	Filter     interface{}
}
type FindOneAndUpdate struct {
	Collection string
	Filter     interface{}
	Update     interface{}
	Result     interface{}
}

type WriteResult struct {
	MatchedCount int64
	DeletedCount int64
}

type Actor struct {
	name   string
	router tree.Router
	db     *mongo.Database
}

func NewActor(name string, db *mongo.Database) *Actor {
	return &Actor{name: name, db: db}
}
func (a *Actor) Name() string { return a.name }
func (a *Actor) OnInit(_ tree.Context) {
	a.router.Register(&InsertOne{}, a.onInsert)
	a.router.Register(&FindOne{}, a.onFindOne)
	a.router.Register(&FindMany{}, a.onFindMany)
	a.router.Register(&Count{}, a.onCount)
	a.router.Register(&UpdateOne{}, a.onUpdate)
	a.router.Register(&ReplaceOne{}, a.onReplace)
	a.router.Register(&DeleteOne{}, a.onDelete)
	a.router.Register(&FindOneAndUpdate{}, a.onFindOneAndUpdate)
}
func (a *Actor) HandleMessage(ctx tree.Context, msg interface{}) { a.router.Route(ctx, msg) }
func (a *Actor) OnStop(_ tree.Context)                           {}

func respond(ctx tree.Context, value interface{}, err error) {
	if envelope := ctx.RequestEnvelope(); envelope != nil {
		envelope.Respond(value, err)
	}
}
func dbctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 8*time.Second)
}

func (a *Actor) onInsert(ctx tree.Context, raw interface{}) {
	m := raw.(*InsertOne)
	c, cancel := dbctx()
	defer cancel()
	r, err := a.db.Collection(m.Collection).InsertOne(c, m.Doc)
	if err != nil {
		respond(ctx, nil, err)
		return
	}
	respond(ctx, r.InsertedID, nil)
}
func (a *Actor) onFindOne(ctx tree.Context, raw interface{}) {
	m := raw.(*FindOne)
	c, cancel := dbctx()
	defer cancel()
	err := a.db.Collection(m.Collection).FindOne(c, m.Filter).Decode(m.Result)
	respond(ctx, m.Result, err)
}
func (a *Actor) onFindMany(ctx tree.Context, raw interface{}) {
	m := raw.(*FindMany)
	c, cancel := dbctx()
	defer cancel()
	cursor, err := a.db.Collection(m.Collection).Find(c, m.Filter)
	if err == nil {
		defer cursor.Close(c)
		err = cursor.All(c, m.Results)
	}
	respond(ctx, m.Results, err)
}
func (a *Actor) onCount(ctx tree.Context, raw interface{}) {
	m := raw.(*Count)
	c, cancel := dbctx()
	defer cancel()
	n, err := a.db.Collection(m.Collection).CountDocuments(c, m.Filter)
	respond(ctx, n, err)
}
func (a *Actor) onUpdate(ctx tree.Context, raw interface{}) {
	m := raw.(*UpdateOne)
	c, cancel := dbctx()
	defer cancel()
	opts := options.UpdateOne()
	if m.Upsert {
		opts.SetUpsert(true)
	}
	r, err := a.db.Collection(m.Collection).UpdateOne(c, m.Filter, m.Update, opts)
	if err != nil {
		respond(ctx, nil, err)
		return
	}
	respond(ctx, WriteResult{MatchedCount: r.MatchedCount}, nil)
}
func (a *Actor) onReplace(ctx tree.Context, raw interface{}) {
	m := raw.(*ReplaceOne)
	c, cancel := dbctx()
	defer cancel()
	r, err := a.db.Collection(m.Collection).ReplaceOne(c, m.Filter, m.Replacement)
	if err != nil {
		respond(ctx, nil, err)
		return
	}
	respond(ctx, WriteResult{MatchedCount: r.MatchedCount}, nil)
}
func (a *Actor) onDelete(ctx tree.Context, raw interface{}) {
	m := raw.(*DeleteOne)
	c, cancel := dbctx()
	defer cancel()
	r, err := a.db.Collection(m.Collection).DeleteOne(c, m.Filter)
	if err != nil {
		respond(ctx, nil, err)
		return
	}
	respond(ctx, WriteResult{DeletedCount: r.DeletedCount}, nil)
}
func (a *Actor) onFindOneAndUpdate(ctx tree.Context, raw interface{}) {
	m := raw.(*FindOneAndUpdate)
	c, cancel := dbctx()
	defer cancel()
	err := a.db.Collection(m.Collection).FindOneAndUpdate(c, m.Filter, m.Update).Decode(m.Result)
	respond(ctx, m.Result, err)
}

func Request(actor string, message interface{}) (interface{}, error) {
	pid, ok := tree.Lookup(actor)
	if !ok {
		return nil, errors.New("mongo actor is unavailable")
	}
	value, err := tree.Request(pid, message).AwaitTimeout(10 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("mongo rpc: %w", err)
	}
	return value, nil
}
