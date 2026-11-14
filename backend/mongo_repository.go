package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PlaybookRepository struct {
	collection *mongo.Collection
}

func NewPlaybookRepository(ctx context.Context, uri, dbName string) (*PlaybookRepository, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}
	return &PlaybookRepository{
		collection: client.Database(dbName).Collection("playbooks"),
	}, nil
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
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}