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

type ToolRepository struct {
	collection *mongo.Collection
}

func NewToolRepository(db *mongo.Database) *ToolRepository {
	return &ToolRepository{
		collection: db.Collection("tools"),
	}
}

func (r *ToolRepository) EnsureBuiltinTools(ctx context.Context) error {
	now := time.Now()

	// Migrate legacy built-in "jira_ticket" document to "ticket" so UI displays only "Ticket".
	var legacyTool ToolDefinition
	if err := r.collection.FindOne(ctx, bson.M{"_id": "jira_ticket", "is_builtin": true}).Decode(&legacyTool); err == nil {
		count, countErr := r.collection.CountDocuments(ctx, bson.M{"_id": "ticket"})
		if countErr != nil {
			return countErr
		}
		if count == 0 {
			legacyTool.ID = "ticket"
			if strings.TrimSpace(legacyTool.Name) == "" || strings.EqualFold(strings.TrimSpace(legacyTool.Name), "Jira Ticket") {
				legacyTool.Name = "Ticket"
			}
			legacyTool.UpdatedAt = now
			if _, insertErr := r.collection.InsertOne(ctx, legacyTool); insertErr != nil && !mongo.IsDuplicateKeyError(insertErr) {
				return insertErr
			}
		}
		if _, delErr := r.collection.DeleteOne(ctx, bson.M{"_id": "jira_ticket", "is_builtin": true}); delErr != nil {
			return delErr
		}
	} else if !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}

	for _, tool := range DefaultBuiltinTools() {
		tool.CreatedAt = now
		tool.UpdatedAt = now
		filter := bson.M{"_id": tool.ID}
		update := bson.M{"$setOnInsert": tool}
		if _, err := r.collection.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true)); err != nil {
			return err
		}
		// Backfill URL parameter metadata on pre-existing built-in docs that predate params support.
		backfillFilter := bson.M{
			"_id":    tool.ID,
			"params": bson.M{"$exists": false},
		}
		backfillUpdate := bson.M{
			"$set": bson.M{
				"http_method":  tool.HTTPMethod,
				"payload_mode": tool.PayloadMode,
				"endpoint":     tool.Endpoint,
				"params":       tool.Params,
			},
		}
		if _, err := r.collection.UpdateOne(ctx, backfillFilter, backfillUpdate); err != nil {
			return err
		}
		if tool.Auth != nil {
			authBackfillFilter := bson.M{
				"_id":  tool.ID,
				"auth": bson.M{"$exists": false},
			}
			authBackfillUpdate := bson.M{
				"$set": bson.M{"auth": tool.Auth},
			}
			if _, err := r.collection.UpdateOne(ctx, authBackfillFilter, authBackfillUpdate); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *ToolRepository) ListAll(ctx context.Context) ([]ToolDefinition, error) {
	opts := options.Find().SetSort(bson.D{{Key: "is_builtin", Value: -1}, {Key: "name", Value: 1}})
	cursor, err := r.collection.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	items := []ToolDefinition{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *ToolRepository) ListEnabled(ctx context.Context) ([]ToolDefinition, error) {
	opts := options.Find().SetSort(bson.D{{Key: "is_builtin", Value: -1}, {Key: "name", Value: 1}})
	cursor, err := r.collection.Find(ctx, bson.M{"enabled": true}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	items := []ToolDefinition{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *ToolRepository) GetByID(ctx context.Context, id string) (*ToolDefinition, error) {
	var tool ToolDefinition
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&tool)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrToolNotFound
		}
		return nil, err
	}
	return &tool, nil
}

func (r *ToolRepository) Create(ctx context.Context, input ToolDefinition) (ToolDefinition, error) {
	tool, err := NormalizeAndValidateTool(input, true)
	if err != nil {
		return ToolDefinition{}, err
	}

	now := time.Now()
	tool.IsBuiltin = false
	tool.CreatedAt = now
	tool.UpdatedAt = now

	_, err = r.collection.InsertOne(ctx, tool)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ToolDefinition{}, ErrToolAlreadyExists
		}
		return ToolDefinition{}, err
	}
	return tool, nil
}

func (r *ToolRepository) Update(ctx context.Context, id string, input ToolDefinition) (ToolDefinition, error) {
	existing, err := r.GetByID(ctx, id)
	if err != nil {
		return ToolDefinition{}, err
	}

	tool, err := NormalizeAndValidateTool(input, false)
	if err != nil {
		return ToolDefinition{}, err
	}

	tool.ID = existing.ID
	tool.IsBuiltin = existing.IsBuiltin
	tool.CreatedAt = existing.CreatedAt
	tool.UpdatedAt = time.Now()

	if _, err := r.collection.ReplaceOne(ctx, bson.M{"_id": id}, tool); err != nil {
		return ToolDefinition{}, err
	}
	return tool, nil
}

func (r *ToolRepository) Delete(ctx context.Context, id string) error {
	existing, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.IsBuiltin {
		return ErrBuiltinToolProtected
	}
	res, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrToolNotFound
	}
	return nil
}
