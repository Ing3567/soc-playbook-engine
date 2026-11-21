package main

import (
	"context"
	"errors"
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
	versions   *mongo.Collection
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
	db := client.Database(dbName)
	return &PlaybookRepository{
		client:     client,
		dbName:     dbName,
		collection: db.Collection("playbooks"),
		versions:   db.Collection("playbook_versions"),
	}, nil
}

func (r *PlaybookRepository) EnsureIndexes(ctx context.Context) error {
	idx := mongo.IndexModel{
		Keys:    bson.D{{Key: "playbook_id", Value: 1}, {Key: "version", Value: -1}},
		Options: options.Index().SetUnique(true),
	}
	_, err := r.versions.Indexes().CreateOne(ctx, idx)
	return err
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
		if items[i].Version <= 0 {
			items[i].Version = 1
		}
		for j := range items[i].Nodes {
			items[i].Nodes[j].Tool = NormalizeLegacyToolID(items[i].Nodes[j].Tool)
		}
	}
	return items, nil
}

func (r *PlaybookRepository) GetByID(ctx context.Context, id string) (*PlaybookDocument, error) {
	var doc PlaybookDocument
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrPlaybookNotFound
		}
		return nil, err
	}
	if doc.Version <= 0 {
		doc.Version = 1
	}
	for j := range doc.Nodes {
		doc.Nodes[j].Tool = NormalizeLegacyToolID(doc.Nodes[j].Tool)
	}
	return &doc, nil
}

func (r *PlaybookRepository) SaveOne(ctx context.Context, doc PlaybookDocument) (PlaybookDocument, error) {
	prevVersion := 0
	if trimmedID := strings.TrimSpace(doc.ID); trimmedID != "" {
		if existing, err := r.GetByID(ctx, trimmedID); err == nil {
			prevVersion = existing.Version
		}
	}
	versionedDoc, snapshot := PreparePlaybookVersion(doc, prevVersion, time.Now())
	if _, err := r.collection.ReplaceOne(ctx, bson.M{"_id": versionedDoc.ID}, versionedDoc, options.Replace().SetUpsert(true)); err != nil {
		return PlaybookDocument{}, err
	}
	if r.versions != nil {
		_, _ = r.versions.ReplaceOne(ctx, bson.M{"_id": snapshot.ID}, snapshot, options.Replace().SetUpsert(true))
	}
	return versionedDoc, nil
}

func (r *PlaybookRepository) ListVersions(ctx context.Context, playbookID string) ([]PlaybookVersionSnapshot, error) {
	opts := options.Find().SetSort(bson.D{{Key: "version", Value: -1}})
	cursor, err := r.versions.Find(ctx, bson.M{"playbook_id": playbookID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	items := []PlaybookVersionSnapshot{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PlaybookRepository) DeleteOne(ctx context.Context, id string) error {
	res, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrPlaybookNotFound
	}
	if r.versions != nil {
		_, _ = r.versions.DeleteMany(ctx, bson.M{"playbook_id": id})
	}
	return nil
}
