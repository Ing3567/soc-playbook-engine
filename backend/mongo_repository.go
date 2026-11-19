package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ErrPlaybookNotFound is returned when a playbook with the given ID does not exist.
var ErrPlaybookNotFound = errors.New("playbook not found")

type PlaybookRepository struct {
	client     *mongo.Client
	dbName     string
	collection *mongo.Collection
}

func NewPlaybookRepository(ctx context.Context, uri, dbName string) (*PlaybookRepository, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return &PlaybookRepository{
		client:     client,
		dbName:     dbName,
		collection: client.Database(dbName).Collection("playbooks"),
	}, nil
}

// Database returns the underlying MongoDB database handle.
func (r *PlaybookRepository) Database() *mongo.Database {
	return r.client.Database(r.dbName)
}

// Ping checks that MongoDB is still reachable (used by /api/health).
func (r *PlaybookRepository) Ping(ctx context.Context) error {
	return r.client.Ping(ctx, nil)
}

func (r *PlaybookRepository) ListAll(ctx context.Context) ([]PlaybookDocument, error) {
	opts := options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}})
	cursor, err := r.collection.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	items := []PlaybookDocument{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	for i := range items {
		for j := range items[i].Nodes {
			items[i].Nodes[j].Tool = NormalizeLegacyToolID(items[i].Nodes[j].Tool)
		}
	}
	return items, nil
}

func (r *PlaybookRepository) SaveOne(ctx context.Context, doc PlaybookDocument) (PlaybookDocument, error) {
	if strings.TrimSpace(doc.Title) == "" {
		doc.Title = "Untitled SOP"
	}
	if strings.TrimSpace(doc.ID) == "" {
		doc.ID = fmt.Sprintf("sop_%d", time.Now().UnixNano())
	}
	doc.UpdatedAt = time.Now()
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": doc.ID}, doc, options.Replace().SetUpsert(true))
	return doc, err
}

func (r *PlaybookRepository) DeleteOne(ctx context.Context, id string) error {
	res, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrPlaybookNotFound
	}
	return nil
}
