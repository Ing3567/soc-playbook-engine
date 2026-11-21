package main

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type ScenarioRepository struct {
	collection *mongo.Collection
}

func NewScenarioRepository(db *mongo.Database) *ScenarioRepository {
	return &ScenarioRepository{
		collection: db.Collection("scenarios"),
	}
}

func (r *ScenarioRepository) EnsureBuiltinScenarios(ctx context.Context) error {
	now := time.Now()
	for _, s := range DefaultBuiltinScenarios() {
		s.CreatedAt = now
		s.UpdatedAt = now
		filter := bson.M{"_id": s.ID}
		update := bson.M{"$setOnInsert": s}
		if _, err := r.collection.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true)); err != nil {
			return err
		}
	}
	return nil
}

func (r *ScenarioRepository) ListAll(ctx context.Context) ([]Scenario, error) {
	opts := options.Find().SetSort(bson.D{{Key: "is_builtin", Value: -1}, {Key: "updated_at", Value: -1}, {Key: "title", Value: 1}})
	cursor, err := r.collection.Find(ctx, bson.D{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	items := []Scenario{}
	if err := cursor.All(ctx, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *ScenarioRepository) GetByID(ctx context.Context, id string) (*Scenario, error) {
	var item Scenario
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&item)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrScenarioNotFound
		}
		return nil, err
	}
	return &item, nil
}

func (r *ScenarioRepository) Create(ctx context.Context, input Scenario) (Scenario, error) {
	s, err := NormalizeAndValidateScenario(input, true)
	if err != nil {
		return Scenario{}, err
	}
	now := time.Now()
	s.IsBuiltin = false
	s.CreatedAt = now
	s.UpdatedAt = now

	if _, err := r.collection.InsertOne(ctx, s); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return Scenario{}, ErrScenarioAlreadyExists
		}
		return Scenario{}, err
	}
	return s, nil
}

func (r *ScenarioRepository) Update(ctx context.Context, id string, input Scenario) (Scenario, error) {
	existing, err := r.GetByID(ctx, id)
	if err != nil {
		return Scenario{}, err
	}
	s, err := NormalizeAndValidateScenario(input, false)
	if err != nil {
		return Scenario{}, err
	}
	s.ID = existing.ID
	s.IsBuiltin = existing.IsBuiltin
	s.CreatedAt = existing.CreatedAt
	s.UpdatedAt = time.Now()

	if _, err := r.collection.ReplaceOne(ctx, bson.M{"_id": id}, s); err != nil {
		return Scenario{}, err
	}
	return s, nil
}

func (r *ScenarioRepository) Delete(ctx context.Context, id string) error {
	existing, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if existing.IsBuiltin {
		return ErrBuiltinScenarioProtected
	}
	res, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return ErrScenarioNotFound
	}
	return nil
}
